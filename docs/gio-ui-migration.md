# ECC UI 迁移交接文档 — Gio 骨架 → 补完前端

> **读者**:负责补完 ECC 前端的 AI/开发者(以下称"接手方")。
> **状态**:Gio 骨架已完成并验证(窗口生命周期、托盘/单实例对接、数据轮询、首页参考实现)。
> **你的任务**:按第 9 节的轮次清单补完 曲线编辑器、预设卡与对话框、设置页按钮、关于页。
> 本文档自足:旧 Win32 实现已删除且从未进过 git,所有规格以本文档为准。

---

## 1. 快速上手

```bash
# 开发预览(免管理员、免单实例锁、模拟数据、配置写入临时目录):
go run ./cmd/ui-preview
# 预览进程自带 pprof: http://127.0.0.1:9977/debug/pprof/

# 全量验证(每轮任务完成后必须全绿):
go vet ./...
go test -race -count=1 ./...
go build -ldflags="-s -w -H windowsgui" -o ecc.exe ./cmd/fan-controller/

# 真机运行(需要管理员, 会请求 UAC; --simulate 不碰硬件):
./ecc.exe --simulate --skip-admin
```

当前 `internal/ui` 文件布局:

| 文件 | 内容 |
|---|---|
| `window_windows.go` | Window 结构、Gio 事件循环、轮询、托盘对接、动作回调(onModeSwitch/onApplyMode/onStrategyChange)、内存软上限 |
| `winapi_windows.go` | WM_CLOSE 子类化拦截(隐藏到托盘)、深色标题栏、FindWindow 激活 |
| `theme_windows.go` | 全部颜色/几何/字体 token(接手方只用这里的 token,禁止新调色) |
| `controls_windows.go` | kit(绘制工具+文本样式)、uiButton/uiSwitch/uiSlider/uiSegmented、addClickArea |
| `home_windows.go` | 框架(顶栏/侧栏/内容分发)、首页参考实现、设置页/关于页占位、Toast |
| `curve.go` | ValidateCurve/formatTemp/formatSpeed/formatRPM/tempColor(纯逻辑,无构建标签) |
| `window_other.go` | 非 Windows stub(保持不动) |
| `ui_test.go` | ValidateCurve 等测试 |

---

## 2. 架构与数据管道(已实现,只需理解)

- **数据流**:1 秒 ticker(`pollLoop`)调 `service.State()` 存入 `w.snapshot`(atomic.Pointer)→ `win.Invalidate()` → Gio 帧回调 `frame()` 读快照渲染。**没有任何 push/订阅机制**。
- **写回**:UI 动作直接调 `service.Update/ApplyPreset/...`(同步、线程安全),成功后立刻 `w.service.State()` 刷新快照。参考 `submitUpdate()`。
- **动作回调**都定义在 `window_windows.go`,首页已示范完整写法(开关/滑块/分段)。
- **Toast**:`w.notify(msg, isError)`,3 秒自动消失,渲染在 `layoutToast`(窗口顶部居中悬浮)。
- **本地暂存状态**:滑块拖动未落盘值存 `w.pendingManual`(UI goroutine 专属字段,无锁)。`w.needInvalidate=true` 用于帧内触发的状态变化要求下一帧。
- **窗口生命周期**:`Start(show)` 起 goroutine 跑 Gio 循环;`Show()` 给托盘唤出;`Close()` 置 `forceClose` 后 `Perform(system.ActionClose)`;`ActivateExisting()` 按标题找窗(单实例二次启动)。
- **关闭到托盘**:`installCloseInterceptor` 用 Gio `Win32ViewEvent` 给出的 HWND 子类化 wndproc,拦截 WM_CLOSE → `ShowWindow(SW_HIDE)`。**不要动这套机制,也不要改窗口标题**(单实例激活与托盘唤出依赖标题 `"ECC 风扇控制器"`)。
- **线程模型**:除上述字段外,所有 UI 状态(`widgets/tab/strategyItems` 等)只在 Gio 事件循环 goroutine 访问,不加锁。托盘在主线程跑自己的 Win32 消息循环,与 Gio 互不干扰。

### 硬性约束(违反会复发已修过的 bug)

1. **注册任何点击区域必须先 clip**:`addClickArea(gtx, &click)`(controls_windows.go)。裸调 `click.Add(gtx.Ops)` 会导致区域无界——一次点击命中窗口内所有控件(已踩坑)。
2. **shaper 必须禁用系统字体扫描**:`newShaper` 里 `text.NoSystemFonts()`。开启会导致 (a) 全系统字体解析、常驻 +150MB;(b) 与 Gio 窗口初始化并发时偶发窗口创建卡死(已踩坑)。
3. **中文字体**:集合里显式加载 msyh.ttc 并按 Normal/Medium/Bold 三权重登记同一 Face(theme_windows.go)。新增字体同样要遵守;不要加载第二个 CJK 字重文件(每个约 +70MB 常驻)。
4. `debug.SetMemoryLimit(96MiB)` 已内建(window_windows.go init),不要移除。
5. 对外 API 签名不许变:`New(service, onExit, hideOnClose) / Start(show) / Show() / Close() / ActivateExisting()`(cmd/fan-controller/main.go 依赖)。

### Gio v0.10.2 API 备忘(与网上旧教程不同)

- `material.NewTheme()` **无参数**;自定义字体走 `th.Shaper` + `th.Face`。
- `clip.UniformRRect(rect, radiusInt).Op(gtx.Ops)` / `.Path(gtx.Ops)`;圆用 `clip.Ellipse`(没有 `clip.Circle`);描边 `clip.Stroke{Path, Width}.Op()`。
- 权重常量:`font.Normal/Medium/Bold`(不是 `WeightBold`)。
- 指针光标:`pointer.CursorPointer.Add(gtx.Ops)`(没有 CursorNameOp)。
- `layout.Stack{Alignment: layout.N}`;`layout.W/E/Center`。
- `gesture.Click` → `KindClick` 为完成态点击;`widget.Float` 有 `Dragging()/Update(gtx)`,`Layout(gtx, axis, marginDp)` 以最小约束为区域。
- 事件循环:`win.Event()` switch `app.FrameEvent` / `app.DestroyEvent` / `app.Win32ViewEvent`。

---

## 3. 设计 Token(theme_windows.go,全部已平移)

**调色板**:ColorTopbar #06447F、ColorSidebar #1D1F21、ColorSidebarSelected #282C30、ColorSidebarHover #222528、ColorMainBg #27292B、ColorControlCard #064786、ColorControlBorder #0B5AA8、ColorCurveCard #001526、ColorCurveBorder #00284A、ColorCardPreset #1A2630、ColorTextPrimary #E8EDF2、ColorTextSecondary #9DA8B3、ColorTextMuted #73808E、ColorAccentGreen #00E85A、ColorAccentHover #33FF7A、ColorDangerRed #E53935、ColorWarningYellow #F59E0B、ColorBorderSubtle #30363C、ColorCurveGrid #0C263E、ColorCurveLine #A0AFBE、ColorInputBg #10161C、ColorInputBorder #283C50、ColorSliderTrack #123048、ColorToastBg #14191E。

**内联色(旧实现散落各处,已收编为 token,语义见注释)**:ColorOnAccent #0A190F(绿底深字)、ColorItemCardBg #043464 / ColorItemCardBd #0F4B87(状态卡内嵌格)、ColorPresetActive #143041(激活预设底)、ColorBtnSecondary #2D3744 / ColorBtnSecondaryB #41505F / ColorBtnSecondHov #374455、ColorBtnOutlineBd #506E8C / ColorBtnOutlineHov #0F5596、ColorGreenOutHov #003219、ColorDangerBg #3C1419、ColorSwitchOffBg #37414B / ColorSwitchOffKnob #BEC8D2 / ColorSwitchOnKnob #0F1914、ColorSegOuterBg #0F1E30 / ColorSegOuterBd #233C5A / ColorSegHover #19304B、ColorToastErrText #FFA0A0、ColorChartBg #021220、ColorTopbarShadow #04284B、ColorThumbBorder #E6F5EB、ColorSliderDisTrk #232D37 / ColorSliderDisThm #505A64 / ColorSliderDisBd #646E78。
悬停色:Outline 按钮 hover 边框变 ColorAccentGreen;Danger hover 变红底白字;Primary hover 变 ColorAccentHover。

**几何(dp)**:顶栏 48、侧栏 88、导航项 64(内缩左右 6,项间距 8)、内容 inset 16、卡片圆角 8/按钮与分段 6/徽标与数值框 4/Toast 8、控制卡高 176(旧 160,为容纳数据格加高)、三卡列间距 14(窄屏 <800dp 时纵向堆叠)、曲线卡高 230、5 点数值框高 30 间距 8、预设卡 220×76 间距 12、FAB 42(距右下边 32)、Toast 距顶 20 最小宽 220。

**字体**(sp):Title 17 Bold、Section 15 Bold、CardTitle 14 Medium、Body 12 Normal、Caption 10 Normal、ValueBig 20 Bold、ValueMid 15 Bold(状态卡数据格用 ValueMid)、等宽 Consolas(FaceMono,曲线刻度可用)。kit 已封装:`k.title/section/cardTitle/body/caption/valueBig/valueMid`。

**图标**:顶栏风扇图标、侧栏 home/settings/about 图标已按旧矢量线绘平移(home_windows.go 顶部),FAB 的 "+" 用两条线画即可。

---

## 4. 页面结构(3 个侧栏标签,NavTab: TabHome/TabSettings/TabAbout)

**框架**:顶栏(风扇图标 + "ECC Fan Control" + "硬件风扇控制台")/ 侧栏三项(控制台/系统设置/关于,选中态:圆角 6 深底 + 最左 3dp 绿色竖条;悬停:浅底)/ 内容区。

### 4.1 控制台页(首页,骨架已实现)

1. Section "Controls · 控制台" → 三卡横排:
   - **状态卡** "实时硬件状态",右上 "更新: HH:MM:SS";2×2 内嵌格:CPU 温度(副"核心传感器")/ GPU 温度(副"核心传感器")/ 目标温度(副"策略计算值",颜色按 <60 绿 <80 黄 ≥80 红)/ 风扇输出(绿色,副"实际: N RPM"或"RPM: 不可用")。
   - **模式卡** "调速模式" + 右上 Switch(44×22);描述(自动=绿色"自动曲线调速"/手动="手动锁定转速");"手动设定转速: N%";Slider(手动时可用);BtnGreenOutline "应用模式设置"。**已接线**(动作 1/2/3)。
   - **策略卡** "温度策略" + 描述文字 + SegmentedControl(4 项)。**已接线**(动作 4)。描述映射(strategyDesc):weighted="综合考量 CPU(70%) 与 GPU(30%) 进行加权平衡";max="直接取 CPU 与 GPU 中当前最高温度控制风扇";cpu="仅根据 CPU 温度变化调节风扇";gpu="仅根据 GPU 独立显卡温度调节风扇"。
2. Section "Curves · 5 点温度/转速曲线",标题行右侧两按钮:**BtnPrimary "保存并应用曲线"**、**BtnOutline "重置曲线"**(尺寸约 110×24,待你接线,见动作 5/6)→ 曲线卡(你的曲线编辑器,见第 7 节)→ 下方 5 个只读数值框 `#N: X°C → Y%`(应改为显示编辑器当前曲线,而非已保存曲线)。
3. Section "Presets · 场景预设" → 预设卡横排 + 右下角 FAB "+"(见第 8 节)。当前是只读占位。

### 4.2 系统设置页(骨架为静态占位,待你补按钮)

标题 "系统集成与运行设置"(Title);500×180 卡片圆角 8:
- Section "开机自启动"
- Body 状态文字:"当前状态:已禁用" / "当前状态:已启用 (通过任务计划程序免 UAC 启动)"(取 `st.Startup`)
- **BtnSecondary "切换自启动"**(约 140×34,待接线,动作 10)
- Caption(ColorTextMuted,可多行)托盘说明:"托盘特性:关闭主窗口将自动最小化到系统右下角托盘,双击托盘图标即可随时唤出控制台。"

### 4.3 关于页(骨架已实现,直接保留)

标题 "关于 ECC Fan Controller";540×200 卡片,Body 多行:
```
ECC 风扇控制器

版本:1.0.0 (纯 Go + Gio 自绘桌面引擎)
架构:直接操作笔记本 EC 寄存器,硬件级闭环温度曲线控制
特性:0 浏览器组件、0 WebView2 依赖、超轻工作集内存占用

遵循开源协议分发与硬件保护规范。
```

---

## 5. 控件规格(controls_windows.go 已有,按需复用/微调)

| 控件 | 要点 |
|---|---|
| Button | 5 样式 Primary/Secondary/Outline/Danger/GreenOutline,圆角 6,CardTitle 字号,悬停变色(色值见 token 区) |
| Switch | 44×22,圆角=高/2;开=绿底 + 深色球(ColorSwitchOnKnob)靠右,关=灰底 + 浅球靠左 |
| Slider | 轨道 6dp 圆角 3,已选区间绿色,thumb r=9(hover/拖动变 ColorAccentHover + 白边,禁用灰);命中区横向外扩 10dp(已用 widget.Float 实现) |
| Segmented | 外框深蓝圆角 6,选中段=绿底深字,悬停=浅蓝底 |
| Toast | 顶部居中悬浮,圆角 8 边框 2(成功绿/错误红),错误文字 ColorToastErrText,前缀 "✓ "/"✕ ",持续 3s(已实现,直接用 `w.notify`) |
| 输入框(新) | 预设对话框用:圆角 4,底 ColorInputBg,边 ColorInputBorder,聚焦边 ColorInputActive(=强调绿);用 `widget.Editor` + 单行,提交走 Enter 或"确定"按钮 |

---

## 6. 曲线编辑器规格(第 1 轮任务)

数据模型:固定 5 点(`controller.CurvePoints`),`controller.Point = [2]float64{temp, speed}`;温度 30–100(`CurveTempMin/Max`),转速 0–100(`CurveSpeedMin/Max`)。校验用 `ValidateCurve`(curve.go,已有测试)。

**渲染**(占位代码在 `layoutCurveCard`,重写为完整实现;建议新建 `curve_view_windows.go`):
- 卡片:ColorCurveCard 底 + ColorCurveBorder 边,圆角 8,高 230dp。
- 图表区 inset:左 40 / 右 20 / 上 24 / 下 30;图表底色 ColorChartBg。
- 横向网格:转速 0/25/50/75/100%,左侧刻度 "N%"(Caption,Mono 可选,ColorTextMuted)。
- 纵向网格:温度 30/45/60/75/90/100°C,底部刻度 "N°C"。
- 折线 2dp ColorCurveLine;控制点 r=5dp 填充 ColorAccentGreen 同色 2dp 边;hover/拖拽中 r=7、填充 ColorAccentHover、白边。
- 悬停提示框:`#N: X°C / Y%`,底色 rgb(15,28,42)、绿边、圆角 4,靠近边缘时翻转防出界。
- 校验错误:红色文字前缀 "⚠️ " 画在图表顶部(`ValidateCurve` 返回的 message)。

**交互**:
- 仅可拖拽既有 5 点,不可增删。按下命中半径 12dp;拖拽中实时更新本地曲线(不落盘)。
- 约束:该点温度必须落在 前一点+1.0 与 后一点-1.0 之间(严格递增 ≥1°C),转速夹 0–100;温度精度 0.1°C(四舍五入),转速取整。
- 拖拽用 Gio `gesture.Drag`(参考 uiSlider 的 widget.Float 思路;或在卡片区域自己处理 pointer 事件)。
- **注意硬性约束 1**:任何 pointer 区域注册都要先 clip。

**接线(编辑器实例存 `widgetsState`,跨帧保持状态)**:
- 本地编辑曲线 `editedCurve []controller.Point` 初始 = `st.Config.Curve`;5 个数值框显示 editedCurve;预设应用/恢复成功后同步重置 editedCurve。
- "保存并应用曲线" = 动作 5;"重置曲线" = 动作 6(见第 9 节 Toast 文案)。

---

## 7. 预设系统(第 2 轮任务)

- 内置 3 个(controller 包 `Presets`):`silent` 静音、`balanced` 平衡、`performance` 性能;自定义 key 为 `custom1、custom2…`(`app/service.go newPresetKey`)。数据已通过 `st.Config.Presets`(map)提供,展示按 key 字母序。
- **内置判定**:key ∈ {silent, balanced, performance}。内置卡显示 "恢复",自定义卡显示 "删除"。
- **当前生效徽标**:策略相同且 5 点完全相等(骨架已有 `isPresetActive`,数据每秒刷新自动更新)。
- **预设卡**(220×76,圆角 8):ColorCardPreset 底;激活时 ColorPresetActive 底 + 绿边;名称 CardTitle;右上 "当前生效" 绿底徽标(56×18 圆角 4,深字);底部三按钮(56×24):应用(BtnPrimary)/保存(BtnSecondary)/恢复(BtnOutline)或删除(BtnDanger)。
- **FAB "+"**:右下角 42dp 圆形 ColorAccentGreen(hover 变 ColorAccentHover),白色/深色 "+" 线条。
- **新建预设对话框**:改为**窗口内模态**(不要用原生 Win32 弹窗):遮罩(半透明黑)+ 居中 360×180 卡片(ColorControlCard 圆角 8),标题 "新建自定义预设",提示 "请输入新预设的名称:",`widget.Editor` 单行输入(≤128 字符),按钮 "确定"(BtnPrimary)/"取消"(BtnSecondary);空名/全空格不提交;Enter=确定,Esc=取消。确认后 = 动作 9。
- 模态实现建议:`frame()` 最外层 Stack 再叠一层全屏遮罩 + `layout.Center` 卡片;模态打开期间用 `w.modalOpen` 标志控制。

---

## 8. 动作与 Toast 文案总表(全部通过 `w.notify`,错误为 isError=true)

| # | 触发 | 写回 | 成功 Toast | 失败 Toast |
|---|---|---|---|---|
| 1 | 模式卡 Switch | `service.Update{Curve, Strategy, ManualEnabled 取反, ManualSpeed}` | 手动:"已切换为手动模式 (N%)";自动:"已切换为自动曲线调速" | "切换模式失败: "+err |
| 2 | Slider 拖动 | 仅本地 pendingManual,不落盘 | 无 | 无 |
| 3 | "应用模式设置" | `service.Update{..., ManualEnabled, ManualSpeed}` | "调速模式已更新生效" | "保存模式失败: "+err |
| 4 | 策略分段切换 | `service.Update{Strategy: 新key, ...}` | "温度策略已切换为: "+key | "策略更新失败: "+err |
| 5 | "保存并应用曲线" | `ValidateCurve` 失败→红 Toast 不写回;成功→`service.Update{Curve: 编辑器当前曲线, ...}` | "风扇曲线已成功保存并下发" | "曲线校验失败: "+err / "曲线保存失败: "+err |
| 6 | "重置曲线" | 不写回;编辑器重置为 st.Config.Curve | "已重置为当前保存的曲线" | 无 |
| 7 | 曲线点拖拽 | 仅本地,松手不自动保存 | 无 | 无 |
| 8a | 预设卡 "应用" | `service.ApplyPreset(key)`;成功后编辑器曲线=新配置 | "已应用预设: "+label | "应用预设失败: "+err |
| 8b | 预设卡 "保存" | 先 `ValidateCurve`(失败→"保存失败: 曲线参数有误")→ `service.Update` → `service.SavePreset(key)` | "已将当前曲线保存到预设: "+label | "保存到预设失败: "+err |
| 8c | 预设卡 "恢复"(内置) | `service.RestorePreset(key)` | "已恢复出厂默认预设: "+label | "恢复预设失败: "+err |
| 8d | 预设卡 "删除"(自定义) | `service.DeletePreset(key)` | "已删除自定义预设: "+label | "删除预设失败: "+err |
| 9 | FAB → 对话框确认 | 先 ValidateCurve+Update 当前曲线(失败静默),再 `service.AddPreset(name)` | "已新建预设: "+name | "新建预设失败: "+err |
| 10 | "切换自启动" | `service.SetStartup(!st.Startup)`,返回 actual | actual=true:"开机自启动已开启";false:"开机自启动已关闭" | "设置开机自启动失败: "+err |

动作 1–4 的回调已在 window_windows.go(onModeSwitch/onApplyMode/onStrategyChange),5–10 照同样模式写。

---

## 9. 你的任务轮次与验收标准

**第 1 轮 — 曲线编辑器**
1. 新建 `curve_view_windows.go`(或并入现有文件),完整实现第 6 节渲染 + 拖拽。
2. 首页接线动作 5/6 与两个按钮;5 个数值框显示编辑器当前曲线;校验错误显示在图表顶部。
3. 验收:`go vet/test/build` 全绿;ui-preview 中拖点流畅、约束生效(相邻点 ≥1°C)、保存后 Toast 正确、重置生效;托盘/隐藏关闭不受影响。

**第 2 轮 — 预设**
1. 预设卡完整实现(8a–8d)+ FAB + 模态新建对话框(9)。
2. 应用/恢复/保存后编辑器曲线与徽标自动刷新(轮询快照已覆盖,注意重置本地 editedCurve)。
3. 验收:同上流程;新建自定义预设出现在卡列;删除自定义卡生效;内置卡无删除按钮。

**第 3 轮 — 设置页 + 关于页收尾**
1. "切换自启动"按钮(动作 10)+ 状态文字随 actual 刷新。
2. 关于页文案按第 4.3 节定稿(骨架已有,核对即可)。
3. 验收:全绿 + 预览确认。

**通用验收(每轮)**:`go vet ./... && go test -race -count=1 ./...` 全绿;`go run ./cmd/ui-preview` 无警告;真机 `--simulate` 下窗口/托盘/关闭到托盘/再次启动激活单实例均正常;不新增第三方依赖;不改 `cmd/fan-controller/main.go`。

---

## 10. 已知差异与后续优化(记录,不阻塞)

- 旧实现窗口 1080×740 物理像素;现为 1080×740 dp(高 DPI 下窗口更大,150% 屏会被 Windows 最大化,已接受)。最小尺寸 920×620 dp(旧版无)。
- 内容区超高时(小窗口)曲线/预设区会被裁掉——旧版同样如此;如要滚动条可用 `layout.List` 包裹首页(可选优化)。
- 状态卡高从 160 → 176dp(修复旧版数值与标签重叠)。
- 内存:Go 堆常驻 ~92MB(雅黑字形急切解析)+ 原生 D3D/驱动;工作集实测 ~185-220MB(旧 GDI 版 ~11MB)。GOMEMLIMIT 96MiB 已内建。若需进一步压缩:构建期用 fonttools 子集化 msyh.ttc(仅常用 3500 字 + UI 文案)可把 CJK 常驻降到 ~10MB,但用户自定义预设名含生僻字会变豆腐块——需权衡。
- 旧实现的模拟模式顶栏副标题 " (模拟模式)" 后缀从未真正启用(旧代码写死 false);app.State 未暴露 simulate 信息,维持现状。
- Windows 11 Mica 材质需要非不透明窗口背景,与现有深色主题冲突,未启用;如需启用研究 DWMWA_SYSTEMBACKDROP_TYPE + 移除 ColorMainBg 平涂。
