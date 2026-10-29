# 编译 GmwmiCli.exe 到 assets/。
# 使用 Windows 自带的 .NET Framework 4.x 编译器，无需安装 .NET SDK。
$ErrorActionPreference = 'Stop'

$cliDir = $PSScriptRoot
$repoRoot = (Resolve-Path (Join-Path $cliDir '..\..')).Path
$framework = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319'
$csc = Join-Path $framework 'csc.exe'
$systemManagement = Join-Path $framework 'System.Management.dll'

if (-not (Test-Path $csc)) {
    throw "未找到 csc.exe: $csc"
}
if (-not (Test-Path $systemManagement)) {
    throw "未找到 System.Management.dll: $systemManagement"
}

$source = Join-Path $cliDir 'GmwmiCli.cs'
$output = Join-Path $repoRoot 'assets\GmwmiCli.exe'

& $csc /nologo /target:exe /platform:x64 /optimize+ `
    "/reference:$systemManagement" `
    "/out:$output" `
    $source

if ($LASTEXITCODE -ne 0) {
    throw "csc 编译失败，退出码 $LASTEXITCODE"
}

Write-Host "已生成: $output"
