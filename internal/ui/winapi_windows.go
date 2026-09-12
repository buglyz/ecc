//go:build windows

package ui

import (
	"syscall"
	"sync/atomic"
	"unsafe"
)

// 本文件只保留 Gio 无法覆盖的少量窗口互操作:
// WM_CLOSE 拦截(关闭隐藏到托盘)、深色标题栏、托盘唤出时的恢复置前、单实例激活。

const (
	wmClose = 0x0010

	swHide    = 0
	swShow    = 5
	swRestore = 9

	gWlpWndProc = ^uintptr(3) // GWLP_WNDPROC = -4

	dwmwaUseImmersiveDarkMode = 20
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	pFindWindowW         = user32.NewProc("FindWindowW")
	pShowWindow          = user32.NewProc("ShowWindow")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pIsWindowVisible     = user32.NewProc("IsWindowVisible")
	pIsIconic            = user32.NewProc("IsIconic")
	pGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	pCallWindowProcW     = user32.NewProc("CallWindowProcW")

	dwmapi                 = syscall.NewLazyDLL("dwmapi.dll")
	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

func utf16Ptr(value string) *uint16 {
	pointer, _ := syscall.UTF16PtrFromString(value)
	return pointer
}

func findWindowByTitle(title string) uintptr {
	hwnd, _, _ := pFindWindowW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(title))))
	return hwnd
}

func windowVisible(hwnd uintptr) bool {
	v, _, _ := pIsWindowVisible.Call(hwnd)
	return v != 0
}

func windowMinimized(hwnd uintptr) bool {
	v, _, _ := pIsIconic.Call(hwnd)
	return v != 0
}

func setDarkTitleBar(hwnd uintptr) {
	enable := uint32(1)
	// 属性 20 在 Win10 20H1+ / Win11 可用; 失败时尝试旧值 19 (Win10 1809-1909)。
	for _, attr := range []uintptr{dwmwaUseImmersiveDarkMode, 19} {
		r, _, _ := pDwmSetWindowAttribute.Call(hwnd, attr, uintptr(unsafe.Pointer(&enable)), 4)
		if r == 0 {
			return
		}
	}
}

// showAndRaise 恢复显示窗口并置于前台, 供托盘菜单/双击与单实例激活使用。
func showAndRaise(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	switch {
	case windowMinimized(hwnd):
		pShowWindow.Call(hwnd, swRestore)
	case !windowVisible(hwnd):
		pShowWindow.Call(hwnd, swShow)
	}
	pSetForegroundWindow.Call(hwnd)
}

// Gio 不处理 WM_CLOSE(默认链路 DefWindowProc → DestroyWindow), 且无否决机制,
// 因此"关闭隐藏到托盘"必须子类化窗口过程拦截 WM_CLOSE。
var (
	subWndProcCallback = syscall.NewCallback(subWndProc)
	interceptedProc    atomic.Uintptr
	activeWindowGlobal atomic.Pointer[Window]
)

// installCloseInterceptor 子类化指定窗口 (hwnd 来自 Gio Win32ViewEvent, 不会找错)。
// startHidden 用于"启动即最小化到托盘"配置 (Gio 无法创建时即隐藏, 只能事后隐藏)。
// 失败时静默放弃, 关闭行为退回 Gio 默认(直接退出)。
func installCloseInterceptor(w *Window, hwnd uintptr, startHidden bool) {
	old, _, _ := pGetWindowLongPtrW.Call(hwnd, gWlpWndProc)
	if old == 0 {
		return
	}
	interceptedProc.Store(old)
	activeWindowGlobal.Store(w)
	pSetWindowLongPtrW.Call(hwnd, gWlpWndProc, subWndProcCallback)

	setDarkTitleBar(hwnd)

	if startHidden {
		pShowWindow.Call(hwnd, swHide)
	}
}

func subWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	orig := interceptedProc.Load()
	w := activeWindowGlobal.Load()
	if orig != 0 && w != nil && msg == wmClose && w.hideOnClose && !w.forceClose.Load() {
		pShowWindow.Call(hwnd, swHide)
		return 0
	}
	ret, _, _ := pCallWindowProcW.Call(orig, hwnd, uintptr(msg), wParam, lParam)
	return ret
}
