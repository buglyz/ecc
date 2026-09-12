# ECC Fan Controller - 安装脚本
# 用法：直接右键"用 PowerShell 运行"即可，脚本会自动请求管理员权限。
#       或在管理员 PowerShell 中执行：.\install.ps1

param(
    [string]$InstallPath = ""
)

$ErrorActionPreference = "Stop"

# 判断当前是否由资源管理器右键启动（这种窗口在脚本结束时会立即关闭）。
$runFromExplorer = $false
try {
    $parentId = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId
    $parentName = (Get-CimInstance Win32_Process -Filter "ProcessId=$parentId" -ErrorAction SilentlyContinue).Name
    if ($parentName -match "explorer") { $runFromExplorer = $true }
} catch { }

# 判断当前是否已是管理员。
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

# 非管理员：右键启动时自动提权重启；终端启动时给出明确提示后退出。
if (-not $isAdmin) {
    if ($runFromExplorer) {
        # 重新以管理员身份启动一个新 PowerShell 窗口执行本脚本，结束保留窗口便于查看结果。
        $scriptPath = $MyInvocation.MyCommand.Definition
        $argList = "-NoExit -ExecutionPolicy Bypass -File `"$scriptPath`""
        if ($InstallPath) { $argList += " -InstallPath `"$InstallPath`"" }
        try {
            Start-Process -FilePath "powershell.exe" -Verb RunAs -ArgumentList $argList
        } catch {
            Write-Host "错误: 用户拒绝了管理员权限请求，无法继续安装。" -ForegroundColor Red
            Write-Host ""
            Write-Host "按回车键关闭此窗口..." -ForegroundColor Yellow
            [void](Read-Host)
        }
        exit
    } else {
        Write-Host "错误: 请以管理员身份运行此脚本。" -ForegroundColor Red
        Write-Host "  方式 1: 右键此脚本 -> 用 PowerShell 运行 (会自动弹 UAC 提权)" -ForegroundColor White
        Write-Host "  方式 2: 在管理员 PowerShell 中执行: .\install.ps1" -ForegroundColor White
        exit 1
    }
}

try {
Write-Host "=== ECC Fan Controller 安装程序 ===" -ForegroundColor Cyan
Write-Host ""

# 安装到 Program Files，确保高权限自启动不会执行用户可写目录中的文件。
$programFilesRoot = [System.IO.Path]::GetFullPath([Environment]::GetFolderPath([Environment+SpecialFolder]::ProgramFiles)).TrimEnd([char[]]@('\', '/'))
if ([string]::IsNullOrWhiteSpace($InstallPath)) {
    $InstallPath = Join-Path $programFilesRoot "ECC"
}
$InstallPath = [System.IO.Path]::GetFullPath($InstallPath).TrimEnd([char[]]@('\', '/'))
$programFilesPrefix = $programFilesRoot + [System.IO.Path]::DirectorySeparatorChar
if (-not $InstallPath.StartsWith($programFilesPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    Write-Host "错误: 安装目录必须位于 $programFilesRoot 下。" -ForegroundColor Red
    throw "安装目录非法"
}

# 切换到脚本所在目录，避免相对路径找不到必需文件。
Set-Location -LiteralPath (Split-Path -Parent $MyInvocation.MyCommand.Path) | Out-Null

# 检查必需文件
$requiredFiles = @("ecc.exe", "assets/ec-probe.exe", "assets/LibreHardwareMonitorLib.dll")
$missing = @()
foreach ($file in $requiredFiles) {
    if (-not (Test-Path $file)) {
        $missing += $file
    }
}

if ($missing.Count -gt 0) {
    Write-Host "错误: 缺少必需文件，请确保在解压后的目录中运行此脚本：" -ForegroundColor Red
    foreach ($file in $missing) {
        Write-Host "  - $file" -ForegroundColor Red
    }
    throw "缺少必需文件"
}

# 创建安装目录
Write-Host "安装目录: $InstallPath" -ForegroundColor Green
if (Test-Path $InstallPath) {
    Write-Host "目标目录已存在，将覆盖现有文件..." -ForegroundColor Yellow
} else {
    New-Item -ItemType Directory -Path $InstallPath -Force | Out-Null
}

# 复制文件
Write-Host "正在复制文件..." -ForegroundColor Green
Copy-Item -Path "ecc.exe" -Destination $InstallPath -Force
Copy-Item -Path "assets" -Destination $InstallPath -Recurse -Force
$targetPath = Join-Path $InstallPath "ecc.exe"

# 仅迁移已有任务，不擅自为用户创建开机自启。
$taskName = "风扇控制"
$existingTask = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Write-Host "正在迁移现有开机自启任务..." -ForegroundColor Green
    & schtasks.exe /Change /TN $taskName /TR ('"{0}"' -f $targetPath) | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "无法将任务计划 $taskName 迁移到受保护安装目录。"
    }
}

# 创建桌面快捷方式
$desktopPath = [Environment]::GetFolderPath("Desktop")
$shortcutPath = Join-Path $desktopPath "ECC Fan Controller.lnk"
Write-Host "正在创建桌面快捷方式..." -ForegroundColor Green

$WScriptShell = New-Object -ComObject WScript.Shell
$shortcut = $WScriptShell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = $targetPath
$shortcut.WorkingDirectory = $InstallPath
$shortcut.Description = "笔记本风扇控制器"
$shortcut.Save()

Write-Host ""
Write-Host "=== 安装完成 ===" -ForegroundColor Green
Write-Host ""
Write-Host "安装路径: $InstallPath" -ForegroundColor Cyan
Write-Host "桌面快捷方式: $shortcutPath" -ForegroundColor Cyan
Write-Host ""
Write-Host "使用说明:" -ForegroundColor Yellow
Write-Host "  1. 双击桌面快捷方式启动程序" -ForegroundColor White
Write-Host "  2. 原生控制窗口会自动打开" -ForegroundColor White
Write-Host "  3. 在控制窗口中可以设置开机自启、调整风扇曲线等" -ForegroundColor White
Write-Host ""
Write-Host "配置文件位置: $env:LOCALAPPDATA\FanController\" -ForegroundColor Cyan
Write-Host "日志文件位置: $env:LOCALAPPDATA\FanController\fan_controller.log" -ForegroundColor Cyan
Write-Host ""

$startNow = "Y"
try { $startNow = Read-Host "是否立即启动程序？(Y/n)" } catch { }
if ($startNow -ne "n" -and $startNow -ne "N") {
    Write-Host "正在启动..." -ForegroundColor Green
    Start-Process -FilePath $targetPath
}

Write-Host ""
Write-Host "提示: 如需卸载，请运行 uninstall.ps1 脚本" -ForegroundColor Yellow

} catch {
    Write-Host ""
    Write-Host "安装失败: $($_.Exception.Message)" -ForegroundColor Red
} finally {
    # 右键运行场景：留住窗口，避免脚本结束就闪退看不到结果。
    # 提权重启的新窗口已用 -NoExit 启动，这里只在原始窗口暂停。
    if ($runFromExplorer) {
        Write-Host ""
        Write-Host "按回车键关闭此窗口..." -ForegroundColor Yellow
        [void](Read-Host)
    }
}
