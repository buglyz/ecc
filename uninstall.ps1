# ECC Fan Controller - 卸载脚本
# 用法：直接右键"用 PowerShell 运行"即可，脚本会自动请求管理员权限。
#       或在管理员 PowerShell 中执行：.\uninstall.ps1 [-KeepConfig]

param(
    [string]$InstallPath = "",
    [switch]$KeepConfig = $false
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
        $scriptPath = $MyInvocation.MyCommand.Definition
        $argList = "-NoExit -ExecutionPolicy Bypass -File `"$scriptPath`""
        if ($InstallPath) { $argList += " -InstallPath `"$InstallPath`"" }
        if ($KeepConfig) { $argList += " -KeepConfig" }
        try {
            Start-Process -FilePath "powershell.exe" -Verb RunAs -ArgumentList $argList
        } catch {
            Write-Host "错误: 用户拒绝了管理员权限请求，无法继续卸载。" -ForegroundColor Red
            Write-Host ""
            Write-Host "按回车键关闭此窗口..." -ForegroundColor Yellow
            [void](Read-Host)
        }
        exit
    } else {
        Write-Host "错误: 请以管理员身份运行此脚本。" -ForegroundColor Red
        Write-Host "  方式 1: 右键此脚本 -> 用 PowerShell 运行 (会自动弹 UAC 提权)" -ForegroundColor White
        Write-Host "  方式 2: 在管理员 PowerShell 中执行: .\uninstall.ps1" -ForegroundColor White
        exit 1
    }
}

try {
Write-Host "=== ECC Fan Controller 卸载程序 ===" -ForegroundColor Cyan
Write-Host ""

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
$localAppDataRoot = [System.IO.Path]::GetFullPath([Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData))
$legacyInstallPath = [System.IO.Path]::GetFullPath((Join-Path $localAppDataRoot "ecc"))
$configPath = [System.IO.Path]::GetFullPath((Join-Path $localAppDataRoot "FanController"))

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
Write-Host "  2. 删除开机自启任务计划 (风扇控制)" -ForegroundColor White
Write-Host "  3. 删除 Program Files 安装目录和旧版用户目录安装" -ForegroundColor White
Write-Host "  4. 删除桌面快捷方式" -ForegroundColor White
if (-not $KeepConfig) {
    Write-Host "  5. 删除配置文件和日志 ($configPath)" -ForegroundColor White
} else {
    Write-Host "  5. 保留配置文件和日志 (使用了 -KeepConfig 参数)" -ForegroundColor Green
}
Write-Host ""

$confirm = "N"
try { $confirm = Read-Host "确认卸载？(y/N)" } catch { }
if ($confirm -ne "y" -and $confirm -ne "Y") {
    Write-Host "已取消卸载。" -ForegroundColor Green
    throw "用户取消"
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
$taskName = "风扇控制"
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
$installPaths = @($InstallPath)
if (-not [string]::Equals($InstallPath, $legacyInstallPath, [System.StringComparison]::OrdinalIgnoreCase)) {
    $installPaths += $legacyInstallPath
}
foreach ($path in $installPaths) {
    if (Test-Path -LiteralPath $path) {
        try {
            Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction Stop
            Write-Host "  已删除: $path" -ForegroundColor Green
        } catch {
            Write-Host "  警告: 删除安装目录失败: $_" -ForegroundColor Yellow
            Write-Host "  请手动删除: $path" -ForegroundColor Yellow
        }
    } else {
        Write-Host "  安装目录不存在，跳过: $path" -ForegroundColor Gray
    }
}

# 4. 删除桌面快捷方式
Write-Host "[4/5] 删除桌面快捷方式..." -ForegroundColor Cyan
$desktopPath = [Environment]::GetFolderPath("Desktop")
$shortcutPath = Join-Path $desktopPath "ECC Fan Controller.lnk"
if (Test-Path $shortcutPath) {
    Remove-Item -LiteralPath $shortcutPath -Force
    Write-Host "  已删除: $shortcutPath" -ForegroundColor Green
} else {
    Write-Host "  快捷方式不存在，跳过" -ForegroundColor Gray
}

# 5. 删除配置文件和日志
Write-Host "[5/5] 处理配置文件和日志..." -ForegroundColor Cyan
if ($KeepConfig) {
    Write-Host "  保留配置文件: $configPath" -ForegroundColor Green
} else {
    if (Test-Path $configPath) {
        try {
            Remove-Item -LiteralPath $configPath -Recurse -Force -ErrorAction Stop
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

} catch {
    if ($_.Exception.Message -ne "用户取消") {
        Write-Host ""
        Write-Host "卸载失败: $($_.Exception.Message)" -ForegroundColor Red
    }
} finally {
    if ($runFromExplorer) {
        Write-Host ""
        Write-Host "按回车键关闭此窗口..." -ForegroundColor Yellow
        [void](Read-Host)
    }
}
