# ECC Fan Controller - 安装脚本
# 用法：在 PowerShell 中以管理员身份运行此脚本

param(
    [string]$InstallPath = "$env:LOCALAPPDATA\ecc"
)

$ErrorActionPreference = "Stop"

Write-Host "=== ECC Fan Controller 安装程序 ===" -ForegroundColor Cyan
Write-Host ""

# 检查是否以管理员身份运行
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "警告: 未以管理员身份运行。某些功能（如开机自启）可能需要管理员权限。" -ForegroundColor Yellow
    Write-Host "建议右键 PowerShell -> 以管理员身份运行，然后重新执行此脚本。" -ForegroundColor Yellow
    Write-Host ""
    $continue = Read-Host "是否继续安装？(y/N)"
    if ($continue -ne "y" -and $continue -ne "Y") {
        exit 0
    }
}

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
    exit 1
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

# 创建桌面快捷方式
$desktopPath = [Environment]::GetFolderPath("Desktop")
$shortcutPath = Join-Path $desktopPath "ECC Fan Controller.lnk"
Write-Host "正在创建桌面快捷方式..." -ForegroundColor Green

$WScriptShell = New-Object -ComObject WScript.Shell
$shortcut = $WScriptShell.CreateShortcut($shortcutPath)
$shortcut.TargetPath = Join-Path $InstallPath "ecc.exe"
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
Write-Host "  2. 浏览器会自动打开控制面板 (http://localhost:8765)" -ForegroundColor White
Write-Host "  3. 在控制面板中可以设置开机自启、调整风扇曲线等" -ForegroundColor White
Write-Host ""
Write-Host "配置文件位置: $env:APPDATA\ecc\" -ForegroundColor Cyan
Write-Host "日志文件位置: $env:APPDATA\ecc\ecc.log" -ForegroundColor Cyan
Write-Host ""

$startNow = Read-Host "是否立即启动程序？(Y/n)"
if ($startNow -ne "n" -and $startNow -ne "N") {
    Write-Host "正在启动..." -ForegroundColor Green
    Start-Process -FilePath (Join-Path $InstallPath "ecc.exe")
}

Write-Host ""
Write-Host "提示: 如需卸载，请运行 uninstall.ps1 脚本" -ForegroundColor Yellow
