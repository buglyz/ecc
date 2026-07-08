# ECC Fan Controller - 卸载脚本
# 用法：在 PowerShell 中以管理员身份运行此脚本

param(
    [string]$InstallPath = "$env:LOCALAPPDATA\ecc",
    [switch]$KeepConfig = $false
)

$ErrorActionPreference = "Stop"

Write-Host "=== ECC Fan Controller 卸载程序 ===" -ForegroundColor Cyan
Write-Host ""

# 检查是否以管理员身份运行
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "警告: 未以管理员身份运行。删除开机自启任务计划可能需要管理员权限。" -ForegroundColor Yellow
    Write-Host ""
}

# 检查安装目录是否存在
if (-not (Test-Path $InstallPath)) {
    Write-Host "未找到安装目录: $InstallPath" -ForegroundColor Yellow
    Write-Host "程序可能已被卸载或安装在其他位置。" -ForegroundColor Yellow
    Write-Host ""
} else {
    Write-Host "安装目录: $InstallPath" -ForegroundColor Green
}

# 提示用户确认
Write-Host "即将卸载 ECC Fan Controller，将执行以下操作：" -ForegroundColor Yellow
Write-Host "  1. 停止正在运行的程序进程" -ForegroundColor White
Write-Host "  2. 删除开机自启任务计划 (FanController)" -ForegroundColor White
Write-Host "  3. 删除安装目录和程序文件" -ForegroundColor White
Write-Host "  4. 删除桌面快捷方式" -ForegroundColor White
if (-not $KeepConfig) {
    Write-Host "  5. 删除配置文件和日志 ($env:APPDATA\ecc\)" -ForegroundColor White
} else {
    Write-Host "  5. 保留配置文件和日志 (使用了 -KeepConfig 参数)" -ForegroundColor Green
}
Write-Host ""

$confirm = Read-Host "确认卸载？(y/N)"
if ($confirm -ne "y" -and $confirm -ne "Y") {
    Write-Host "已取消卸载。" -ForegroundColor Green
    exit 0
}

Write-Host ""
Write-Host "开始卸载..." -ForegroundColor Green
Write-Host ""

# 1. 停止进程
Write-Host "[1/5] 停止程序进程..." -ForegroundColor Cyan
$processes = Get-Process -Name "ecc" -ErrorAction SilentlyContinue
if ($processes) {
    foreach ($proc in $processes) {
        Write-Host "  停止进程 PID=$($proc.Id)..." -ForegroundColor Yellow
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Seconds 2
    Write-Host "  进程已停止" -ForegroundColor Green
} else {
    Write-Host "  未找到运行中的进程" -ForegroundColor Gray
}

# 2. 删除开机自启任务计划
Write-Host "[2/5] 删除开机自启任务计划..." -ForegroundColor Cyan
$taskName = "FanController"
$task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if ($task) {
    try {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction Stop
        Write-Host "  已删除任务计划: $taskName" -ForegroundColor Green
    } catch {
        Write-Host "  警告: 删除任务计划失败 (可能需要管理员权限): $_" -ForegroundColor Yellow
    }
} else {
    Write-Host "  未找到任务计划: $taskName" -ForegroundColor Gray
}

# 3. 删除安装目录
Write-Host "[3/5] 删除安装目录..." -ForegroundColor Cyan
if (Test-Path $InstallPath) {
    try {
        Remove-Item -Path $InstallPath -Recurse -Force -ErrorAction Stop
        Write-Host "  已删除: $InstallPath" -ForegroundColor Green
    } catch {
        Write-Host "  警告: 删除安装目录失败: $_" -ForegroundColor Yellow
        Write-Host "  请手动删除: $InstallPath" -ForegroundColor Yellow
    }
} else {
    Write-Host "  安装目录不存在，跳过" -ForegroundColor Gray
}

# 4. 删除桌面快捷方式
Write-Host "[4/5] 删除桌面快捷方式..." -ForegroundColor Cyan
$desktopPath = [Environment]::GetFolderPath("Desktop")
$shortcutPath = Join-Path $desktopPath "ECC Fan Controller.lnk"
if (Test-Path $shortcutPath) {
    Remove-Item -Path $shortcutPath -Force
    Write-Host "  已删除: $shortcutPath" -ForegroundColor Green
} else {
    Write-Host "  快捷方式不存在，跳过" -ForegroundColor Gray
}

# 5. 删除配置文件和日志
Write-Host "[5/5] 处理配置文件和日志..." -ForegroundColor Cyan
$configPath = "$env:APPDATA\ecc"
if ($KeepConfig) {
    Write-Host "  保留配置文件: $configPath" -ForegroundColor Green
} else {
    if (Test-Path $configPath) {
        try {
            Remove-Item -Path $configPath -Recurse -Force -ErrorAction Stop
            Write-Host "  已删除: $configPath" -ForegroundColor Green
        } catch {
            Write-Host "  警告: 删除配置目录失败: $_" -ForegroundColor Yellow
            Write-Host "  请手动删除: $configPath" -ForegroundColor Yellow
        }
    } else {
        Write-Host "  配置目录不存在，跳过" -ForegroundColor Gray
    }
}

Write-Host ""
Write-Host "=== 卸载完成 ===" -ForegroundColor Green
Write-Host ""

if (-not $KeepConfig) {
    Write-Host "程序已完全卸载。" -ForegroundColor Green
} else {
    Write-Host "程序已卸载，但配置文件已保留在: $configPath" -ForegroundColor Green
    Write-Host "如需完全清理，请手动删除该目录。" -ForegroundColor Yellow
}
Write-Host ""
