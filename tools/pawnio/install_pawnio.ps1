# 安装 PawnIO 2.2.0（WHCP 签名的端口 I/O 驱动）。
# 仅由用户明确授权后运行；安装过程由 winget 处理。
$ErrorActionPreference = 'Continue'
$log = Join-Path $env:TEMP 'pawnio_install.log'
Start-Transcript -Path $log -Force | Out-Null
try {
    Write-Host "Installing PawnIO via winget..."
    winget install --id namazso.PawnIO -e --accept-package-agreements --accept-source-agreements --silent
    Write-Host "WINGET_EXIT=$LASTEXITCODE"
    Get-Service PawnIO -ErrorAction SilentlyContinue | Format-List Name,Status,StartType
} finally {
    Stop-Transcript | Out-Null
}
