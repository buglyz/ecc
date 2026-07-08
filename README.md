# ECC Fan Controller

[![Build and Test](https://github.com/buglyz/ecc/actions/workflows/build.yml/badge.svg)](https://github.com/buglyz/ecc/actions/workflows/build.yml) [![Manual Release](https://github.com/buglyz/ecc/actions/workflows/release.yml/badge.svg)](https://github.com/buglyz/ecc/actions/workflows/release.yml)

笔记本 EC（Embedded Controller）风扇控制器，通过写入 EC 寄存器实现自定义风扇转速曲线。现已重构并搭载全新的现代化 Web UI。

## 由[此项目](https://www.bilibili.com/video/BV1oaaoeFEzY/?share_source=copy_web&vd_source=02adb0cd5f8f9003d535f030aa4f3266)修改而来

## 预览
![PixPin_2026-06-14_18-58-40.png](https://k-vault-39a.pages.dev/file/AgACAgUAAyEGAATjDinyAAMJai6J77BSz8rw8Bt4twWfKI-JOY0AAsUQaxuqN3hViRKhQeE5-f4BAAMCAAN3AAM8BA.png)
![PixPin_2026-06-14_18-58-43.png](https://k-vault-39a.pages.dev/file/AgACAgUAAyEGAATjDinyAAMLai6J89hSBEUPxtFxgOsAAX-gFk9AAALGEGsbqjd4VWGbIgQZAy2qAQADAgADdwADPAQ.png)

## 🌟 核心特性

- 🎨 **现代化玻璃态 Web UI**：采用 Fluent/Glassmorphism 设计美学，支持深色/浅色自适应主题切换及完美的响应式布局。
- 📈 **双轨制风扇曲线编辑器**：支持在可视化画布上直观拖拽控制节点，也支持在下方的数据表格中精确键入温度/转速数值，双向实时同步。
- 📊 **实时硬件监控图表**：动态展示 CPU、GPU 温度与风扇响应趋势，支持自定义回溯时间范围（1 分钟至 480 分钟）。
- ⚙️ **多模式智能调速**：
  - 自动模式：基于多种温度传感策略（加权、取最大值、仅 CPU、仅 GPU）动态计算。
  - 手动模式：一键锁定绝对转速百分比。
- 🕹️ **自定义场景预设**：内置 静音 / 平衡 / 性能 三大预设，支持一键创建、修改、保存个人专属挡位配置。
- 🚀 **系统级无缝集成**：
  - 动态系统托盘图标（实时监控展示与右键快捷菜单）。
  - 开机自启动管理（基于 Windows 任务计划程序，彻底免除 UAC 烦人弹窗）。
  - 无依赖、纯本地单文件运行，启动自动触发管理员提权。
- 🔒 **API 跨域防护**：`/api/*` 端点自动校验 Origin 与 Host 头，拒绝来自其他网站的跨站请求，防止恶意网页篡改风扇配置。

## 运行要求

- Windows 10/11
- 管理员权限（EC 寄存器访问需要内核驱动）
- `assets/` 目录需包含以下文件（随 exe 一起分发）：
  - `ec-probe.exe` — NBFC EC 写入工具
  - `nbfc.exe` — NBFC 主程序
  - `StagWare.FanControl.dll`
  - `NLog.dll`
  - `clipr.dll`
  - `Plugins/` — NBFC 硬件插件
  - `LibreHardwareMonitorLib.dll` — 温度读取

## 使用

双击 `ecc.exe`：
1. 自动请求管理员权限（UAC 提示）
2. 系统托盘出现图标
3. 右键任务栏窗口打开webui

### 命令行参数

```
--port 8765       Web 控制台端口（默认 8765）
--dry-run         仅记录 EC 写入，不实际操作硬件
--simulate        使用模拟温度数据（同时启用 dry-run）
--skip-admin      跳过管理员权限检查
--no-tray         禁用系统托盘图标
--no-browser      启动时不自动打开浏览器
```

## 编译

```bash
go build -ldflags="-s -w -H windowsgui" -o ecc.exe ./cmd/fan-controller/
```

无外部依赖，纯 Go 标准库。

## 🔬 工作原理

本程序的核心是一个「读温度 → 算目标转速 → 写 EC 寄存器」的闭环控制循环。下面从数据流角度详细拆解。

### 整体数据流

```
                    ┌─────────────────────────────────────────────┐
                    │              控制循环 (每秒采样)               │
                    │                                             │
  温度读取           │   ┌──────────┐   温度策略      ┌──────────┐  │   EC 写入
┌──────────┐        │   │ 采样窗口  │─────────────→ │ 曲线插值  │  │  ┌──────────┐
│PowerShell│───────→│   │ (6 次)   │  加权/max/单路 │ 求目标转速 │  │─→│ec-probe  │──→ EC 寄存器
│  + LHM   │ CPU/GPU │   └──────────┘               └──────────┘  │  │  .exe    │   0x2C/0x2D
└──────────┘  温度   │        │                          │        │  └──────────┘
                    │        │        ┌──────────┐      │        │
  RPM 反馈           │        └───────→│ 滞回/漂移 │←─────┘        │
┌──────────┐  实际转速 │                 │ 心跳判断  │              │
│WMI GMWMI │───────→│                 └──────────┘              │
└──────────┘        └─────────────────────────────────────────────┘
                              ↑                    ↓
                         ┌─────────┐         ┌──────────┐
                         │ Web UI  │←───────→│  配置     │
                         │ 曲线编辑 │  HTTP   │ JSON 持久化│
                         └─────────┘   API   └──────────┘
```

### 1. 温度采集（`internal/sensors`）

程序**不直接读硬件传感器**，而是通过一个常驻的 PowerShell 子进程桥接 [LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor)（LHM）：

1. 首次读取时，把内嵌的 PowerShell 脚本写到状态目录并启动 `powershell.exe`，脚本内 `Add-Type` 加载 `LibreHardwareMonitorLib.dll`。
2. 主程序通过 stdin 发送 `read\n`，PowerShell 侧读取所有 CPU/GPU 温度传感器，以 JSON 单行 (`{"cpu":63.5,"gpu":58.0}`) 从 stdout 返回。
3. 主程序设 5 秒读取超时；超时或子进程崩溃即杀掉并重启。
4. **指数退避**：连续失败时重启间隔按 1s→2s→4s…（上限 30s）递增，避免子进程反复崩溃时每秒白白 spawn 一个 `powershell.exe`。读到有效数据后计数清零。

> 为什么用子进程而不是 CGO 直接调 DLL？因为 LHM 是 .NET 程序集，Go 无法直接调用；PowerShell 是 Windows 自带的最省事的 .NET 宿主，且进程隔离让 DLL 崩溃不会拖垮主程序。

### 2. 温度策略（`internal/controller/logic.go` · `CombineTemps`）

一个采样周期含 **6 次采样**（`SamplesPerCycle`，每秒一次）。每次采样都把 CPU/GPU 两路温度按当前策略合并成单个「目标温度」：

| 策略 | 计算方式 |
|------|---------|
| `weighted`（默认） | `0.7 × CPU + 0.3 × GPU` |
| `max` | `max(CPU, GPU)` |
| `cpu` | 仅 CPU |
| `gpu` | 仅 GPU |

周期结束时，对 6 次的目标温度取算术平均，得到本周期的代表温度 `avg`。取平均是为了平滑瞬时尖峰，避免风扇因一次采样毛刺而频繁调速。

### 3. 曲线插值（`internal/controller/logic.go` · `InterpolateCurve`）

风扇曲线是 5 个 `(温度, 转速%)` 控制点。用平均温度 `avg` 做**分段线性插值**求目标转速：

- `avg` 低于最低点温度 → 取最低点转速
- `avg` 高于最高点温度 → 取最高点转速（封顶 100%）
- 落在两点之间 → 线性插值：`s1 + (avg−t1)/(t2−t1) × (s2−s1)`

结果经 `ClampSpeed` 限幅到 `[0, 100]` 并四舍五入为整数百分比。

### 4. 何时才真正写 EC —— 滞回、漂移、心跳（`internal/controller/controller.go`）

算出目标转速后**并不是每周期都写**，写入受多重条件门控，以减少无谓的 EC 操作与风扇转速抖动：

- **滞回 (Hysteresis)**：若本周期平均温度与「上次已提交温度」相差 `< 2°C`（`HysteresisTemp`），视为温度已稳定，跳过写入。防止温度在临界点附近来回摆动导致风扇忽快忽慢。
- **漂移 (Drift)**：若一个周期实际耗时偏离预期（6s + 抖动容差）超过 `LoopDriftTolerance`（系统卡顿、休眠唤醒等），强制写一次校正。
- **心跳 (Heartbeat)**：即使温度稳定，每 `30s`（`HeartbeatInterval`）也强制写一次，防止 EC 因固件看门狗把风扇控制权收回。
- **配置变化**：用户在 Web UI 改了曲线/策略（配置版本号递增），立即强制写入，不等心跳，保证改动秒级生效。
- **手动模式**：直接锁定用户指定的转速百分比，绕过曲线计算。

只有「目标转速变了 **或** 漂移 **或** 心跳到期 **或** 配置变化」时才调用写入。

### 5. 写入 EC 寄存器（`internal/ec/writer.go`）

这是**真正控制风扇的一步**。程序不自己碰内核，而是调用 NBFC 的 `ec-probe.exe`：

```
ec-probe.exe write -v 0x2C 0x32
                    │    │    └─ 值：0x32 = 十进制 50 = 50% 转速
                    │    └────── 寄存器：0x2C = Fan1
                    └─────────── 命令：写入并回读验证 (-v)
```

- `ec-probe.exe` 内部加载 **WinRing0 内核驱动**，通过 IO 端口 `0x62/0x66` 与嵌入式控制器（EC）通信，把一个字节写入指定寄存器。EC 固件读到该寄存器后按百分比驱动风扇 PWM。
- 程序对 **Fan1 (`0x2C`) 和 Fan2 (`0x2D`) 各写一次**，两者都成功才算写入成功。
- **驱动失败检测**：`ec-probe write` 在 WinRing0 驱动加载失败时**仍返回退出码 0**（区别于 read/dump 返回 1）。只看退出码会把「驱动没加载、EC 根本没写进去」误判为成功，导致滞回逻辑以为已生效而不再重试。因此程序会扫描命令输出，命中 `unable to load the winring0 driver` 标志即判为失败，触发托盘告警并在下一周期重试。
- **优雅退出**：程序停止时向两个寄存器写 `0xFF`，把风扇控制权交还给 EC 固件（恢复 BIOS 自动调速），避免退出后风扇卡在某个固定转速。

### 6. 转速反馈（`internal/sensors/gmwmi_rpm.go`）

为了在 UI 上显示风扇**实际转速**（而非仅设定的百分比），程序通过 WMI 的 `RW_GMWMI` 接口（神舟等 Gaming WMI 笔记本）读取一段 `BufferBytes`，从固定偏移解析出小端 16 位 RPM 值（`BufferBytes[0x0C-0x0D]` = CPU 风扇，`[0x10-0x11]` = GPU 风扇）。不可用时优雅降级，UI 不显示 RPM。

### 7. Web UI 与配置（`internal/dashboard` · `internal/config`）

- 内嵌 HTTP 服务器（默认 `127.0.0.1:8765`）提供单页 Web 控制台，前端通过 `go:embed` 打进 exe。
- 前端每秒轮询 `/api/state` 拉取最新温度/转速/历史，用 Canvas 画实时曲线；改曲线时 POST `/api/config`。
- 配置以 JSON 持久化到 `%AppData%\ecc\`，写入采用「临时文件 + 原子重命名」防止写一半断电损坏；加载时若发现损坏会自动备份为 `.corrupted.<时间戳>` 并回退默认配置，同时闪动托盘图标提醒。
- 所有写接口 (`/api/config`、`/api/preset`、`/api/startup`) 校验 `Origin` 头，只允许本机回环来源，防止恶意网页 CSRF 篡改风扇设置。

### 关键常量一览（`internal/controller/constants.go`）

| 常量 | 值 | 含义 |
|------|-----|------|
| `SamplesPerCycle` | 6 | 每个控制周期的采样次数 |
| `SampleInterval` | 1s | 采样间隔（可用 `--interval` 覆盖） |
| `HysteresisTemp` | 2.0°C | 滞回阈值，温差小于此值不调速 |
| `HeartbeatInterval` | 30s | 强制写入间隔（防固件收回控制权） |
| `CPUWeight` | 0.7 | weighted 策略的 CPU 权重 |
| `ECRegFan1 / Fan2` | 0x2C / 0x2D | 风扇转速寄存器 |
| `ECFanRelease` | 0xFF | 释放控制、交还固件 |

## EC 寄存器

- `0x2C` — Fan1 转速（0-100 百分比，0xFF 释放控制）
- `0x2D` — Fan2 转速（同上）

## 项目结构

```
cmd/fan-controller/     程序入口
internal/
  admin/                管理员提权 + DPI 感知
  config/               配置加载/保存/归一化（JSON + pickle 兼容）
  controller/           风扇控制循环核心逻辑
  dashboard/            Web 控制台（HTTP API + 前端）
  ec/                   EC 寄存器写入（调用 ec-probe.exe）
  logging/              日志轮转
  paths/                运行时路径发现
  process/              Windows 隐藏窗口进程属性
  sensors/              温度读取（PowerShell + LibreHardwareMonitor）
  startup/              开机自启动（任务计划程序）
  tray/                 系统托盘图标（Win32 Shell_NotifyIcon）
```

## 📋 重构路线图

- [x] **拆 `pickle.go`** — 将 `config.go` 中 ~500 行 pickle 解析器独立为 `internal/config/pickle.go`，`config.go` 只保留配置业务逻辑
- [x] **前端抽离** — 将 `dashboard.go` 内联的 CSS/HTML/JS 迁移到 `web/` 目录，通过 `go:embed` 嵌入，`dashboard.go` 只留 HTTP handler
- [x] **API 禁止跨域** — `/api/*` 端点校验 `Origin`/`Host` 头，拒绝非 localhost 来源的请求
- [x] **Google Fonts 本地化** — 将外链字体下载到 `web/assets/` 本地引用，离线场景 UI 不崩
- [x] **轮询间隔可配置** — 将 `time.After(time.Second)` 硬编码改为可配置的 `--interval` 参数
- [x] **font 渲染测试** — 为 `tray/font_windows.go` 的 `renderText16` / glyphs 补跨平台可测的纯计算单元测试
- [x] **EC 写入失败告警** — `writeSpeed` 返回 false 时托盘闪动/弹通知，不让用户对控制失效毫不知情
- [x] **温度传感器掉线恢复** — PowerShell 进程连续失败时指数退避重启+告警，避免每秒白启动
- [x] **风扇转速反馈** — 通过 WMI RW_GMWMI 接口读取实际风扇 RPM，Web UI 图表显示实时转速
  - ✅ **神舟笔记本支持**：通过 Gaming WMI (RW_GMWMI) 接口读取 CPU 和 GPU 风扇实际转速
  - ℹ️ **技术细节**：BufferBytes[0x0C-0x0D] = CPU RPM, BufferBytes[0x10-0x11] = GPU RPM

## ⚠️ 硬件兼容性

**风扇控制**：
- ✅ 通过 EC 寄存器 (0x2C, 0x2D) 写入风扇转速百分比
- ⚠️ 注意：部分硬件的 EC 寄存器是只写的，无法读回验证，但写入仍然有效

**风扇转速反馈（RPM）**：
- ✅ **神舟笔记本（已测试）**：通过 WMI RW_GMWMI 接口读取，完美支持
- ⚠️ 其他品牌笔记本可能使用不同的接口，需要适配
- ℹ️ 如果 RPM 读取不可用，程序会自动检测并优雅降级（不显示 RPM 信息）


