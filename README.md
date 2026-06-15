# ECC Fan Controller

[![Build](https://github.com/buglyz/ecc/actions/workflows/build.yml/badge.svg)](https://github.com/buglyz/ecc/actions/workflows/build.yml) [![Manual Release](https://github.com/buglyz/ecc/actions/workflows/release.yml/badge.svg)](https://github.com/buglyz/ecc/actions/workflows/release.yml)

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
- [x] **风扇转速反馈** — 读取实际风扇转速寄存器，Web UI 图表显示目标转速 vs 实际转速对比


