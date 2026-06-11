# FanController

笔记本 EC 风扇控制器，通过写入 EC 寄存器实现自定义风扇转速曲线。

## 功能

- 自动模式：根据 CPU/GPU 温度按曲线自动调速
- 手动模式：锁定指定转速百分比
- 温度策略：加权、取最大值、仅 CPU、仅 GPU
- 预设方案：静音 / 平衡 / 性能 一键切换
- 可拖拽风扇曲线编辑器
- 实时温度与转速图表（带图例、坐标轴）
- 系统托盘图标（动态温度显示 + 右键菜单）
- 开机自启动（任务计划程序，无 UAC 弹窗）
- 自动管理员提权（ShellExecuteW runas）
- Web 控制台（浏览器访问 http://127.0.0.1:8765）
- 深色 / 浅色主题切换
- 配置持久化（JSON，兼容旧版 pickle 格式）

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

双击 `fan-controller.exe`：
1. 自动请求管理员权限（UAC 提示）
2. 系统托盘出现图标
3. 浏览器自动打开控制台

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
go build -ldflags="-s -w -H windowsgui" -o fan-controller.exe ./cmd/fan-controller/
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

## Python 原版

`src/` 目录为 Python 原版源码，`FanController/` 为 PyInstaller 打包版本。
