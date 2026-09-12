//go:build windows

package ui

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"gioui.org/text"

	"github.com/buglyz/ecc/internal/app"
	"github.com/buglyz/ecc/internal/controller"
)

const winTitle = "ECC 风扇控制器"

// CJK 字体字形经 typesetting 急切解析常驻堆上, 收紧 GC 软上限
// 避免堆目标翻倍带来的内存膨胀 (实测可省 100MB+)。
const memorySoftLimit = 96 << 20 // 96 MiB

func init() {
	debug.SetMemoryLimit(memorySoftLimit)
}

const (
	winInitW = 1080
	winInitH = 740
	winMinW  = 920
	winMinH  = 620

	toastDuration = 3 * time.Second
	pollInterval  = time.Second
)

type NavTab int

const (
	TabHome NavTab = iota
	TabSettings
	TabAbout
)

// widgetsState 持有跨帧的控件交互状态; 仅 UI 事件循环 goroutine 访问。
type widgetsState struct {
	navClicks  [3]gesture.Click
	modeSwitch uiSwitch
	applyBtn   uiButton
	segmented  uiSegmented
	slider     uiSlider
}

// toastState 的可见字段由 toastMu 保护 (notify/过期定时器在非 UI goroutine)。
type toastState struct {
	msg      string
	isErr    bool
	visible  bool
	expireAt time.Time
}

type Window struct {
	service     *app.Service
	onExit      func()
	hideOnClose bool
	forceClose  atomic.Bool

	snapshot atomic.Pointer[app.State]
	winRef   atomic.Pointer[gioapp.Window]
	hwnd     atomic.Uintptr

	toastMu sync.Mutex
	toast   toastState

	// ---- 以下字段仅 UI 事件循环 goroutine 访问, 无需加锁 ----
	kit            kit
	widgets        widgetsState
	tab            NavTab
	strategyItems  []SegmentItem
	pendingManual  int // 滑块拖动的未落盘值, -1 表示无
	needInvalidate bool

	ready     chan struct{}
	done      chan struct{}
	closeReq  chan struct{}
	closeOnce sync.Once
	startErr  error
	readyOnce sync.Once
}

var (
	fontOnce     sync.Once
	sharedShaper *text.Shaper
)

func buildKit() kit {
	fontOnce.Do(func() {
		sharedShaper = newShaper(buildFontCollection())
	})
	th := material.NewTheme()
	th.Shaper = sharedShaper
	th.Face = font.Typeface(FaceMain)
	return kit{th: th}
}

func New(service *app.Service, onExit func(), hideOnClose bool) *Window {
	st := service.State()
	w := &Window{
		service:       service,
		onExit:        onExit,
		hideOnClose:   hideOnClose,
		ready:         make(chan struct{}),
		done:          make(chan struct{}),
		closeReq:      make(chan struct{}),
		pendingManual: -1,
	}
	w.snapshot.Store(&st)

	items := make([]SegmentItem, len(controller.Strategies))
	for i, s := range controller.Strategies {
		items[i] = SegmentItem{Key: s.Key, Label: s.Label, Desc: strategyDesc(s.Key)}
	}
	w.strategyItems = items

	w.kit = buildKit()
	return w
}

func (w *Window) Start(show bool) error {
	go w.run(show)
	<-w.ready
	return w.startErr
}

// Show 供托盘菜单/双击唤出主窗口。
func (w *Window) Show() {
	hwnd := w.hwnd.Load()
	if hwnd == 0 {
		hwnd = findWindowByTitle(winTitle)
	}
	showAndRaise(hwnd)
	if win := w.winRef.Load(); win != nil {
		win.Invalidate()
	}
}

// Close 关闭窗口并等待事件循环退出 (由托盘退出触发)。
func (w *Window) Close() {
	w.closeOnce.Do(func() { close(w.closeReq) })
	<-w.done
}

// ActivateExisting 单实例激活: 按标题找到已运行实例的主窗口并前置。
func ActivateExisting() bool {
	hwnd := findWindowByTitle(winTitle)
	if hwnd == 0 {
		return false
	}
	showAndRaise(hwnd)
	return true
}

func (w *Window) run(show bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(w.done)
	defer w.readyOnce.Do(func() { close(w.ready) })
	defer func() {
		if w.onExit != nil {
			w.onExit()
		}
	}()

	win := new(gioapp.Window)
	win.Option(
		gioapp.Title(winTitle),
		gioapp.Size(unit.Dp(winInitW), unit.Dp(winInitH)),
		gioapp.MinSize(unit.Dp(winMinW), unit.Dp(winMinH)),
	)
	w.winRef.Store(win)

	go w.wakeLoop(win)
	go w.pollLoop()

	startHidden := !show
	var ops op.Ops
	for {
		e := win.Event()
		switch e := e.(type) {
		case gioapp.Win32ViewEvent:
			if e.Valid() {
				w.hwnd.Store(e.HWND)
				go installCloseInterceptor(w, e.HWND, startHidden)
			}
		case gioapp.FrameEvent:
			gtx := gioapp.NewContext(&ops, e)
			w.frame(gtx)
			e.Frame(gtx.Ops)
			if w.needInvalidate {
				w.needInvalidate = false
				win.Invalidate()
			}
		case gioapp.DestroyEvent:
			select {
			case <-w.ready:
			default:
				w.startErr = fmt.Errorf("窗口启动失败: %w", e.Err)
			}
			return
		}
	}
}

// wakeLoop 阻塞等待退出请求, 通过 Perform(ActionClose) 唤醒 Gio 事件循环。
// forceClose 标志让 WM_CLOSE 子类化过程放行本次关闭 (否则会被拦截转为隐藏)。
func (w *Window) wakeLoop(win *gioapp.Window) {
	select {
	case <-w.closeReq:
		select {
		case <-w.done:
			return
		default:
		}
		w.forceClose.Store(true)
		win.Perform(system.ActionClose)
	case <-w.done:
	}
}

// pollLoop 每秒拉取一次服务状态并触发重绘 (平移旧实现的 WM_TIMER 轮询模型)。
func (w *Window) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			st := w.service.State()
			w.snapshot.Store(&st)
			if win := w.winRef.Load(); win != nil {
				win.Invalidate()
			}
		case <-w.done:
			return
		}
	}
}

func (w *Window) frame(gtx layout.Context) {
	st := w.snapshot.Load()
	if st == nil {
		st = &app.State{}
	}
	k := &w.kit
	fillRect(gtx, ColorMainBg)

	layout.Stack{Alignment: layout.N}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return w.layoutShell(gtx, k, st)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return w.layoutToast(gtx, k)
		}),
	)
}

// ---------- 动作 (平移自旧 actions_windows.go) ----------

// manualSpeed 当前展示/提交的手动转速: 滑块拖动未落盘值优先。
func (w *Window) manualSpeed(st *app.State) int {
	if w.pendingManual >= 0 {
		return w.pendingManual
	}
	return st.Config.ManualSpeed
}

func (w *Window) notify(msg string, isError bool) {
	w.toastMu.Lock()
	w.toast = toastState{msg: msg, isErr: isError, visible: true, expireAt: time.Now().Add(toastDuration)}
	w.toastMu.Unlock()

	time.AfterFunc(toastDuration+100*time.Millisecond, func() {
		w.toastMu.Lock()
		if time.Now().After(w.toast.expireAt) {
			w.toast.visible = false
		}
		w.toastMu.Unlock()
		if win := w.winRef.Load(); win != nil {
			win.Invalidate()
		}
	})
	if win := w.winRef.Load(); win != nil {
		win.Invalidate()
	}
}

// submitUpdate 提交配置更新, 成功后立即刷新快照; 返回错误供调用方分支处理。
func (w *Window) submitUpdate(u app.Update, okMsg, errPrefix string) error {
	_, err := w.service.Update(u)
	if err != nil {
		w.notify(errPrefix+err.Error(), true)
		return err
	}
	st := w.service.State()
	w.snapshot.Store(&st)
	w.notify(okMsg, false)
	return nil
}

// onModeSwitch 开关切换: 立即写回 (旧动作 #1)。
func (w *Window) onModeSwitch(st *app.State, enabled bool) {
	u := app.Update{
		Curve:         st.Config.Curve,
		Strategy:      st.Config.Strategy,
		ManualEnabled: enabled,
		ManualSpeed:   w.manualSpeed(st),
	}
	if err := w.submitUpdate(u, "", "切换模式失败: "); err != nil {
		return
	}
	if enabled {
		w.notify(fmt.Sprintf("已切换为手动模式 (%d%%)", w.manualSpeed(st)), false)
	} else {
		w.notify("已切换为自动曲线调速", false)
	}
}

// onManualSpeed 滑块拖动: 仅更新本地状态, 落盘由"应用模式设置"触发 (旧动作 #2/#3)。
func (w *Window) onManualSpeed(v int) {
	w.pendingManual = v
	w.needInvalidate = true
}

// onApplyMode 应用模式设置按钮 (旧动作 #3)。
func (w *Window) onApplyMode(st *app.State) {
	u := app.Update{
		Curve:         st.Config.Curve,
		Strategy:      st.Config.Strategy,
		ManualEnabled: st.Config.ManualEnabled,
		ManualSpeed:   w.manualSpeed(st),
	}
	if w.submitUpdate(u, "调速模式已更新生效", "保存模式失败: ") == nil {
		w.pendingManual = -1
	}
}

// onStrategyChange 策略分段切换: 立即写回 (旧动作 #4)。
func (w *Window) onStrategyChange(st *app.State, key string) {
	if key == st.Config.Strategy {
		return
	}
	u := app.Update{
		Curve:         st.Config.Curve,
		Strategy:      key,
		ManualEnabled: st.Config.ManualEnabled,
		ManualSpeed:   w.manualSpeed(st),
	}
	w.submitUpdate(u, "温度策略已切换为: "+key, "策略更新失败: ")
}
