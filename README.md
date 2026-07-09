# ECC Fan Controller

[![Build and Test](https://github.com/buglyz/ecc/actions/workflows/build.yml/badge.svg)](https://github.com/buglyz/ecc/actions/workflows/build.yml) [![Manual Release](https://github.com/buglyz/ecc/actions/workflows/release.yml/badge.svg)](https://github.com/buglyz/ecc/actions/workflows/release.yml)

笔记本 EC（Embedded Controller）风扇控制器：通过写入 EC 寄存器实现自定义风扇转速曲线，搭载现代化玻璃态 Web UI。纯 Go 标准库实现，单文件运行，无外部运行时依赖。

> 由[此 B 站项目](https://www.bilibili.com/video/BV1oaaoeFEzY/?share_source=copy_web&vd_source=02adb0cd5f8f9003d535f030aa4f3266)修改而来。

## 目录

- [预览](#预览)
- [核心特性](#-核心特性)
- [快速开始](#-快速开始)
- [命令行参数](#命令行参数)
- [工作原理](#-工作原理)
- [硬件兼容性](#️-硬件兼容性)
- [从源码编译](#-从源码编译)
- [项目结构](#-项目结构)
- [故障排查](#-故障排查)

## 预览

![预览 1](https://k-vault-39a.pages.dev/file/AgACAgUAAyEGAATjDinyAAMJai6J77BSz8rw8Bt4twWfKI-JOY0AAsUQaxuqN3hViRKhQeE5-f4BAAMCAAN3AAM8BA.png)
![预览 2](https://k-vault-39a.pages.dev/file/AgACAgUAAyEGAATjDinyAAMLai6J89hSBEUPxtFxgOsAAX-gFk9AAALGEGsbqjd4VWGbIgQZAy2qAQADAgADdwADPAQ.png)

## 🌟 核心特性

- 🎨 **现代化玻璃态 Web UI** — Fluent/Glassmorphism 设计，深色/浅色自适应，响应式布局。
- 📈 **双轨制曲线编辑器** — 画布上拖拽控制点，或在表格中精确键入温度/转速，双向实时同步。
- 📊 **实时监控图表** — CPU/GPU 温度与风扇响应趋势，回溯范围 1–480 分钟可调。
- ⚙️ **多模式调速** — 自动模式支持加权 / 取最大值 / 仅 CPU / 仅 GPU 四种温度策略；手动模式一键锁定绝对转速。
- 🕹️ **场景预设** — 内置 静音 / 平衡 / 性能 三挡，支持创建、修改、保存个人挡位。
- 🚀 **系统级集成** — 动态托盘图标、开机自启动（任务计划程序，免 UAC 弹窗）、启动自动提权。
- 🔒 **API 跨域防护** — `/api/*` 端点校验来源，拒绝恶意网页 CSRF 篡改风扇配置。
- 🛡️ **健壮性** — 温度传感器掉线指数退避重启、EC 写入失败托盘告警、配置损坏自动备份恢复。

## 🚀 快速开始

### 运行要求

- Windows 10 / 11（x64）
- 管理员权限（访问 EC 寄存器需加载内核驱动）
- `ecc.exe` 同目录下的 `assets/` 需包含以下随包分发的文件：

  | 文件 | 作用 |
  |------|------|
  | `ec-probe.exe` | NBFC 的 EC 读写工具（核心） |
  | `LibreHardwareMonitorLib.dll` | 读取 CPU/GPU 温度 |
  | `nbfc.exe` | NBFC 主程序 |
  | `StagWare.FanControl.dll` / `NLog.dll` / `clipr.dll` | NBFC 依赖库 |
  | `Plugins/` | NBFC 硬件插件 |

### 安装与使用

1. 从 [Releases](https://github.com/buglyz/ecc/releases) 下载并解压，或右键 `install.ps1` 以管理员身份运行 PowerShell 一键安装。
2. 双击 `ecc.exe` 启动，确认 UAC 提权提示。
3. 系统托盘出现图标，浏览器自动打开控制台 `http://127.0.0.1:8765`。
4. 在控制台中调整风扇曲线、切换预设、设置开机自启动。

> 卸载：运行 `uninstall.ps1`（加 `-KeepConfig` 可保留配置）。

### 命令行参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `--port` | `8765` | Web 控制台端口 |
| `--interval` | `1000` | 采样间隔（毫秒） |
| `--dry-run` | 关 | 仅记录 EC 写入，不操作硬件 |
| `--simulate` | 关 | 使用模拟温度数据（同时启用 dry-run） |
| `--skip-admin` | 关 | 跳过管理员权限检查 |
| `--no-tray` | 关 | 禁用系统托盘图标 |
| `--no-browser` | 关 | 启动时不自动打开浏览器 |

## 🔬 工作原理

本程序的核心是一个「读温度 → 算目标转速 → 写 EC 寄存器」的闭环控制循环。

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
│ec-probe  │───────→│                 └──────────┘              │
│0xB0-0xB3 │        └─────────────────────────────────────────────┘
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

#### 5.1 EC 是什么、为什么要写它

**EC（Embedded Controller，嵌入式控制器）** 是笔记本主板上一颗独立的微控制器，独立于 CPU 运行，负责电源管理、键盘、电池、温度监控和**风扇 PWM 调速**等底层任务。它对外暴露一组 8 位寄存器（共 256 个，地址 `0x00`–`0xFF`），操作系统通过读写这些寄存器与 EC 交互。风扇转速就由其中某几个寄存器的值决定。

#### 5.2 本程序使用的寄存器地址

| 寄存器地址 | 用途 | 取值范围 | 说明 |
|-----------|------|---------|------|
| `0x2C` | **Fan1 转速** | `0x00`–`0x64` (0–100) | 十进制百分比。`0x32`=50%，`0x64`=100% |
| `0x2D` | **Fan2 转速** | `0x00`–`0x64` (0–100) | 同上，第二个风扇 |
| `0x2C` / `0x2D` | **释放控制** | `0xFF` | 写 `0xFF` 交还固件自动调速 |
| `0xB0` / `0xB1` | Fan1 实际转速 (RPM) | 只读 | 低字节 / 高字节，小端组合成 16 位 RPM |
| `0xB2` / `0xB3` | Fan2 实际转速 (RPM) | 只读 | 低字节 / 高字节 |

> ⚠️ **这些地址是特定于本机型的**。不同厂商、不同型号笔记本的 EC 寄存器布局完全不同（`0x2C/0x2D` 是本项目目标机型逆向得到的地址）。换一台笔记本很可能需要重新用 [RWEverything](http://rweverything.com/) 之类工具逆向探测。定义见 `internal/controller/constants.go`：
>
> ```go
> ECRegFan1        = "0x2C"   // Fan1 转速百分比
> ECRegFan2        = "0x2D"   // Fan2 转速百分比
> ECRegFan1RPMLow  = "0xB0"   // Fan1 RPM 低字节
> ECRegFan1RPMHigh = "0xB1"   // Fan1 RPM 高字节
> ECFanRelease     = "0xFF"   // 释放控制
> ```

#### 5.3 一次写入的完整链路

以「设 Fan1 到 50%」为例，从 Go 代码到硬件寄存器的完整调用链：

```
控制循环 writeSpeed(50)
   │  toHex(50) → "0x32"       // 百分比转十六进制字符串
   ▼
ec.Writer.Write(ctx, "0x2C", "0x32")
   │  exec: ec-probe.exe write -v 0x2C 0x32
   ▼
ec-probe.exe (NBFC 工具, .NET)
   │  加载 WinRing0 内核驱动 (WinRing0x64.sys)
   ▼
WinRing0 驱动 (Ring 0 内核态)
   │  通过 IO 端口读写与 EC 握手
   ▼
EC 硬件访问协议 (端口 0x62 / 0x66)
   │  ① 等待 EC 空闲 (读状态端口 0x66，检查 IBF 位)
   │  ② 向命令端口 0x66 写 0x81 (WRITE_EC 命令)
   │  ③ 向数据端口 0x62 写寄存器地址 0x2C
   │  ④ 向数据端口 0x62 写数据 0x32
   ▼
EC 固件
   │  寄存器 0x2C 的值变为 0x32
   ▼
风扇 PWM 控制器 → 风扇以 50% 占空比转动
```

**关于 IO 端口 `0x62` / `0x66`**：这是 ACPI 标准定义的 EC 访问端口，几乎所有 x86 笔记本通用：
- `0x66` — **命令/状态端口**。读它得到状态字节（含 IBF 输入缓冲满、OBF 输出缓冲满标志位）；写它发送命令（`0x80`=读 EC、`0x81`=写 EC）。
- `0x62` — **数据端口**。读写实际的地址和数据字节。

访问这两个端口需要 Ring 0 内核态权限，用户态程序无法直接 `out`/`in`，这正是必须借助 **WinRing0 内核驱动**（一个签名的合法驱动，暴露端口读写能力给用户态）的原因，也是程序必须**以管理员权限运行**才能加载驱动的原因。

#### 5.4 写入策略与容错

- **双风扇都写**：程序对 Fan1 (`0x2C`) 和 Fan2 (`0x2D`) **各写一次**，两者都成功才算本次写入成功（`writeSpeed` 返回 `true`）。
- **写入超时**：每次 `ec-probe.exe` 调用设 5 秒超时（`writeTimeout`），防止驱动挂死拖住控制循环。
- **驱动失败检测**：`ec-probe write` 在 WinRing0 驱动加载失败时**仍返回退出码 0**（区别于 read/dump 返回 1）。只看退出码会把「驱动没加载、EC 根本没写进去」误判为成功，导致滞回逻辑以为已生效而不再重试。因此程序会扫描命令输出，命中 `unable to load the winring0 driver` 标志即判为失败（见 `writeSucceeded`），触发托盘告警并在下一周期重试。
- **只写不回读的机型**：部分笔记本的转速寄存器是**只写**的——写进去有效，但立即回读拿到的仍是旧值或 `0xFF`。因此程序**不依赖回读来确认转速生效**，只依赖 `ec-probe` 命令本身是否成功执行。
- **优雅退出**：程序停止时向两个寄存器写 `0xFF`（`ECFanRelease`），把风扇控制权交还给 EC 固件（恢复 BIOS 自动调速），避免退出后风扇卡在某个固定转速或长期满速。

> 💡 **手动验证写入是否生效**（管理员 PowerShell，在 `assets/` 目录下）：
> ```powershell
> .\ec-probe.exe read 0x2C           # 读当前值
> .\ec-probe.exe write 0x2C 0x32 -v  # 写 50% 并回读验证
> .\ec-probe.exe write 0x2C 0xFF     # 释放控制，交还固件
> ```

### 6. 转速反馈（`internal/ec/reader.go`）

为了在 UI 上显示风扇**实际转速**（而非仅设定的百分比），程序复用同一条 `ec-probe.exe` + WinRing0 路径，读取 EC 寄存器 `0xB0/0xB1`（Fan1）与 `0xB2/0xB3`（Fan2），按小端 16 位组合成 RPM。读到 0 或读取失败时优雅降级，UI 不显示 RPM。

> 这两对寄存器是真机 dump 交叉验证得到的：强制 Fan1 从 0% 拉到 100% 时 `0xB0/0xB1` 组合值从 0 跳到约 3800，空闲态约 5000 rpm，且与机器自带控制台读数一致。早期版本曾假设 RPM 在 `0xD0-0xD3`（错误，恒为常量 99）并试过 WMI `RW_GMWMI` 接口（被动 `SELECT *` 只能拿到全零的静态缓冲，该接口实为需要写命令字节才回填的读写通道），两条路径均已废弃。

### 7. Web UI 与配置（`internal/dashboard` · `internal/config`）

- 内嵌 HTTP 服务器（默认 `127.0.0.1:8765`）提供单页 Web 控制台，前端通过 `go:embed` 打进 exe。
- 前端每秒轮询 `/api/state` 拉取最新温度/转速/历史，用 Canvas 画实时曲线；改曲线时 POST `/api/config`。
- 配置以 JSON 持久化到 `%LOCALAPPDATA%\FanController\`，写入采用「临时文件 + 原子重命名」防止写一半断电损坏；加载时若发现损坏会自动备份为 `.corrupted.<时间戳>` 并回退默认配置，同时闪动托盘图标提醒。
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

## ⚠️ 硬件兼容性

**风扇控制**（写 EC 寄存器 `0x2C` / `0x2D`）：
- ✅ 本项目目标机型已验证：写入立即生效，可用管理员 PowerShell 通过 `ec-probe read` 回读确认。
- ⚠️ EC 寄存器地址因机型而异。换其他笔记本需自行逆向探测正确地址（见 [5.2](#52-本程序使用的寄存器地址)），否则写入无效甚至误触其他功能。
- ⚠️ 部分机型寄存器只写不可回读，程序对此已兼容（不依赖回读判定成功）。

**风扇转速反馈（RPM）**：
- ✅ **本项目目标机型已验证**：读 EC 寄存器 `0xB0`/`0xB1`（Fan1）、`0xB2`/`0xB3`（Fan2），小端 16 位。强制 0%→100% 时读数从 ~0 跳到 ~3800，空闲态 ~5000 rpm，与机器自带控制台一致。
- ⚠️ RPM 寄存器地址同样因机型而异。换机型需重新用满速/停转 dump 对比法定位（见 [5.2](#52-本程序使用的寄存器地址)）。
- ℹ️ RPM 读取不可用（读到 0 或驱动未加载）时程序自动优雅降级（UI 不显示 RPM），不影响调速功能。

## 🛠 从源码编译

```bash
go build -ldflags="-s -w -H windowsgui" -o ecc.exe ./cmd/fan-controller/
```

- 无外部依赖，纯 Go 标准库 + 少量系统调用封装。
- `-H windowsgui` 让程序以 GUI 子系统启动（不弹控制台窗口）。
- 编译后需把 `assets/` 目录放到 `ecc.exe` 同级。

## 📂 项目结构

```
cmd/fan-controller/     程序入口
internal/
  admin/                管理员提权 + DPI 感知
  config/               配置加载/保存/归一化（JSON + pickle 兼容），损坏自动备份
  controller/           风扇控制循环核心逻辑（采样/策略/插值/滞回/心跳）
  dashboard/            Web 控制台（HTTP API + go:embed 前端）
  ec/                   EC 寄存器读写（调用 ec-probe.exe）
  logging/              日志轮转
  paths/                运行时路径发现
  process/              Windows 隐藏窗口进程属性
  sensors/              温度读取（PowerShell + LHM）
  startup/              开机自启动（任务计划程序）
  tray/                 系统托盘图标（Win32 Shell_NotifyIcon）
```

## 🔧 故障排查

| 现象 | 可能原因与排查 |
|------|--------------|
| 风扇转速不变 | 驱动未加载或寄存器地址不对。管理员 PowerShell 跑 `ec-probe read 0x2C`；若报 `unable to load the winring0 driver` 则驱动问题，若读值不随写入变化则地址不匹配本机型。 |
| 启动即退出 | 缺少管理员权限或 `assets/` 文件不全。查看日志 `%LOCALAPPDATA%\FanController\fan_controller.log`。 |
| UI 显示温度为空 | LHM/PowerShell 桥接失败。日志会记录退避重启信息；确认 `LibreHardwareMonitorLib.dll` 存在。 |
| UI 不显示 RPM | 驱动未加载或本机型 RPM 寄存器地址不同（本项目为 `0xB0`–`0xB3`）。属优雅降级，不影响调速。 |
| 托盘图标闪红 | EC 写入失败告警，通常是驱动未加载，见第一行。 |
| 配置丢失/重置 | 配置文件曾损坏并被备份为 `config.json.corrupted.<时间戳>`，可在状态目录找回。 |

日志位置：`%LOCALAPPDATA%\FanController\fan_controller.log`（按大小轮转，保留最近若干份）。
