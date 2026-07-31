# ECC 风扇控制项目只读并行审核报告

- 仓库：`https://github.com/buglyz/ecc`
- 审核提交：`7d356401b0cc77e977989ab361f44960c013c730`
- 日期：2026-08-01
- 约束：只读审核；未修改项目代码，未提交、推送、发布；未执行任何 Windows EXE、安装/卸载脚本或 EC 硬件操作。

## 结论

项目已有配置规范化、请求体大小限制、写失败重试意识、退出释放和较完整单测等良好基础，但当前仍存在会直接影响硬件安全与高权限边界的问题。建议在继续扩功能前，优先处理以下 P0/P1 项。

## P0 / 严重

### 1. 未知机型也会写死的 EC 寄存器

- 位置：`internal/controller/constants.go:7-17`、`cmd/fan-controller/main.go:103-135`
- 现状：固定把 `0x2C/0x2D` 当作双风扇 PWM，启动前没有 SMBIOS 厂商、型号、主板、BIOS 白名单。
- 影响：在非目标机型上可能误写电源、充电或其他 EC 功能。
- 建议：按机型建立硬件档案；未知机型默认拒绝真实写入，只允许 dry-run 和显式探测。

### 2. 自动模式在首次有效温度前先写 30%，温度丢失后也没有 fail-safe

- 位置：`internal/controller/controller.go:201-215,233-279,336-341`
- 现状：启动 1 秒后先写默认 30%，然后才采样；CPU/GPU 均为空时直接 continue，继续保持旧 PWM，不释放、不升安全转速。
- 额外证据：`internal/sensors/powershell.go:110-124,289-300` 会把脚本返回的 `{"cpu":null,"gpu":null}` 当成功，清零失败计数，持续异常时不会重启助手。
- 影响：高温启动、LHM/DLL故障或传感器掉线时，风扇可能长期停留在低速。
- 建议：取得可信温度前不接管 EC；连续无有效温度超过短阈值时立即释放给固件或切安全高转速并告警。

### 3. 管理员进程的本地写 API 无鉴权，Origin 检查可被绕过

- 位置：`internal/dashboard/dashboard.go:102-110,141-155,195-250,320-346`
- 现状：任一本机低权限进程都可 POST 设置手动 0%、曲线和计划任务；无 Origin 请求被放行。DNS rebinding 下攻击域的 Origin 与 Host 可相等，也会通过。
- 影响：形成低权限调用管理员硬件控制面的 confused-deputy 边界。
- 建议：所有写接口强制随机会话凭证；严格限制 Host 为实际回环地址；更稳妥地改用带当前 Windows 用户 ACL 的命名管道。

### 4. 卸载使用强杀，跳过 0xFF 风扇释放

- 位置：`uninstall.ps1:67-75`；对照 `internal/controller/controller.go:150-157,323-333`
- 现状：`Stop-Process -Force` 使 `defer fan.Stop()` 无法执行，还按名称杀死所有 `ecc.exe`。
- 影响：EC 可能保留最后固定 PWM；随后探针又被删除。
- 建议：通过受保护 IPC 请求优雅释放和退出；仅在超时后强杀，并按完整可执行路径筛选。

## P1 / 高

### 5. 单实例保护只依赖端口

- 位置：`cmd/fan-controller/main.go:49-55,120-136`
- 触发：使用不同 `--port`，或 `--port=0`。
- 影响：多个进程并发写同一 EC；一个实例退出写 `0xFF` 会干扰另一个。
- 建议：硬件初始化前获取与端口无关的 Windows 全局命名 Mutex。

### 6. 提权后可从工作目录回退加载 EXE/DLL

- 位置：`internal/paths/paths.go:43-61`、`internal/admin/admin.go:39-48`
- 现状：UAC 重启继承工作目录；依赖候选包含 `wd\assets` 和 `wd`，只做 `os.Stat`。
- 影响：标准资产缺失、便携运行或异常更新时，可能以管理员权限执行/加载用户可写路径中的 `ec-probe.exe` 或 LHM DLL。
- 建议：发布模式仅允许受保护安装根；拒绝重解析点/普通用户可写 ACL，并验证签名或固定哈希。

### 7. 安装脚本未用 `$PSScriptRoot` 锚定安装源

- 位置：`install.ps1:31-36,56-59,100-103`
- 现状：检查和复制的 `ecc.exe`、`assets` 均相对当前工作目录。
- 影响：管理员可能安装并立即启动错误目录中的同名文件。
- 建议：所有源路径从 `$PSScriptRoot` 构造，并使用 `-LiteralPath` 和文件类型检查。

### 8. 预编译二进制不可追溯，CI 直接打包进高权限发行物

- 位置：`assets/`、`.github/workflows/build.yml:40-47`、`.github/workflows/release.yml:37-61`
- 现状：多个 EXE/DLL 无源码提交、固定上游版本、来源清单、SBOM、第三方许可证和 CI 输入哈希验证；关键文件未见 Authenticode。
- 当前关键哈希：
  - `ec-probe.exe`: `17db72d437d52b22733d252d041c6b91a5578788f16f0f9fafcf6b64142a88cd`
  - `LibreHardwareMonitorLib.dll`: `a24c7cbb3d6ca12950e570fbad82778a87c87311cde6218914a283a2c0a04e19`
- 影响：供应链替换可进入管理员/内核驱动路径。
- 建议：固定上游版本与 SHA-256，提供来源清单/SBOM/许可证，CI 验证输入并签名或生成构建证明。

### 9. RPM 反馈没有形成安全闭环

- 位置：`internal/controller/controller.go:182-198,240,314-320`
- 现状：仅读取 Fan1；0 RPM 被转成 nil；Fan2 RPM 常量未使用；RPM 只展示，不参与故障判定。
- 影响：风扇堵转、断线、寄存器错误或 Fan2 故障不会触发安全动作。
- 建议：分别维护 Fan1/Fan2 目标与 RPM；非零目标持续零转时告警并释放/升安全转速。

### 10. 托盘启动/停止存在竞态和挂死窗口

- 位置：`internal/tray/tray_windows.go:162-202,214-220,275-285`、`cmd/fan-controller/main.go:180-197`
- 现状：`running=true` 早于 hwnd/nid 初始化；Stop 可能向 hwnd=0 投递并永久等 stopCh；部分字段未在同一锁下初始化和访问。
- 影响：快速退出、SIGTERM 或 Win32 初始化失败时可能无法退出，正常风扇释放也无法完成。
- 建议：增加可锁存的 stopping/ready/done 状态；Win32 状态只在托盘线程串行修改。

### 11. Release 标签与资产不是不可变的

- 位置：`.github/workflows/release.yml:4-9,37-40,66-73`
- 现状：tag 输入未严格校验并直接插入 PowerShell；未拒绝已有 tag/release，资产默认可能被覆盖；未明确绑定 `target_commitish`。
- 影响：同一版本标签下的 ZIP/校验和可能改变，标签、构建提交和 VERSION 信息可能不一致。
- 建议：校验 SemVer，通过环境变量传值；仅允许受保护主分支；已有版本立即失败；关闭覆盖并启用 immutable release。

## P2 / 中

### 12. 双风扇部分写成功被压缩为单一布尔值

- 位置：`internal/controller/controller.go:288-320`
- 影响：Fan1 成功、Fan2 失败时硬件已分裂，但 UI 仍显示旧统一速度，可能等到心跳才修复。
- 建议：分别记录两路应用状态；部分失败后强制重试或回滚/释放成功的一侧。

### 13. 两路释放共用一个 5 秒 context

- 位置：`internal/controller/controller.go:323-333`
- 影响：Fan1 用满 5 秒后，Fan2 会收到已取消 context，且无重试。
- 建议：两路使用独立短 deadline 和少量独立重试。

### 14. 温度传感器按数组位置选值

- 位置：`internal/sensors/powershell.go:275-296`
- 现状：CPU 取倒数第二项，GPU 取第一项，不按稳定名称或最大温度。
- 影响：LHM 枚举顺序变化或存在多个传感器时可能选到较低温度。
- 建议：优先匹配 Package/Core Max/Hot Spot 等稳定名称，并采用经过验证的最大有效值。

### 15. 配置应用与文件保存缺乏跨进程/原子整体语义

- 位置：`internal/dashboard/dashboard.go:363-367`、`internal/config/config.go:105-121`
- 现状：曲线、策略、手动模式分三次 setter；多实例共用固定 `config.json.tmp`。
- 影响：可能短暂观察到混合配置；多实例时丢更新或保存失败。
- 建议：提供单次 `ApplyConfig`；使用唯一临时文件和跨进程锁。

### 16. CI 工具链漂移且 Action 未固定完整 SHA

- 位置：`build.yml:17-31,50`、`release.yml:19-24,67`、`go.mod:3`
- 现状：Go 1.26.5、stable、go 1.22 三处不一致；govulncheck 使用 latest；Action 使用可移动主版本标签。
- 影响：构建不可复现，旧提交重跑可能执行不同第三方代码。
- 建议：统一精确 Go 版本，固定 govulncheck 版本和 Action 40 位提交 SHA。

## 其他工程债务

- Go 单文件超出默认 400 行限制：
  - `internal/config/config.go` 574 行
  - `internal/tray/tray_windows.go` 473 行
  - `internal/dashboard/dashboard.go` 431 行
  - `internal/config/pickle.go` 412 行
- 安装更新为覆盖式合并，无暂存切换和回滚，可能留下新旧混合资产。
- 通用进程名 `ecc` 与任务名 `风扇控制` 可能误伤同名程序/任务。
- README 的“单文件运行、无外部运行时依赖”与实际必须携带多项 EXE/DLL、PowerShell/.NET 的要求不一致。

## 只读验证

- `go vet ./...`：通过。
- `GOMAXPROCS=2 go test -short -p=1 -count=1 ./...`：通过。
- 未把 `go test -race` 计为通过。
- Windows 托盘、UAC、计划任务、PowerShell/LHM 和真实 EC 行为未在真实 Windows/硬件上动态执行。
