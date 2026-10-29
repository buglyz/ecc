# Controlled PawnIO EC write test for QNLXS / SQ131.
# Writes 0x2C/0x2D = 0x64 (100%) for 3 seconds, then unconditionally restores 0xFF/0xFF.
# Based on the MIT-licensed PawnIO interop from 87380623czc/hasee-s8d6-ec-fan-control.

$ErrorActionPreference = 'Stop'
$Module = (Resolve-Path (Join-Path $PSScriptRoot '..\..\assets\pawnio\LpcACPIEC.bin')).Path
$OutFile = Join-Path $env:TEMP 'pawnio_write_test.txt'
$LogFile = Join-Path $env:TEMP 'pawnio_write_test.log'
Set-Content -Path $OutFile -Value '' -Encoding UTF8
Start-Transcript -Path $LogFile -Force | Out-Null

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class PawnIOCtl
{
    public const uint DEVICE_TYPE       = 41394u << 16;
    public const int  FN_NAME_LENGTH    = 32;
    public const uint IOCTL_LOAD_BINARY = DEVICE_TYPE | (0x821u << 2);
    public const uint IOCTL_EXECUTE     = DEVICE_TYPE | (0x841u << 2);

    public const uint GENERIC_READ_WRITE = 0xC0000000u;
    public const uint FILE_SHARE_RW      = 0x00000003u;
    public const uint OPEN_EXISTING      = 3u;
    public const uint FILE_ATTRIBUTE_NORMAL = 0x80u;

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    public static extern IntPtr CreateFileW(string lpFileName, uint dwDesiredAccess, uint dwShareMode,
        IntPtr lpSecurityAttributes, uint dwCreationDisposition, uint dwFlagsAndAttributes, IntPtr hTemplateFile);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool DeviceIoControl(IntPtr hDevice, uint dwIoControlCode,
        byte[] lpInBuffer, uint nInBufferSize, byte[] lpOutBuffer, uint nOutBufferSize,
        out uint lpBytesReturned, IntPtr lpOverlapped);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool CloseHandle(IntPtr hObject);
}
'@

$script:H = [IntPtr]::Zero
$script:ObfUsed = $null

function Open-PawnIO {
    param([string]$ModulePath)
    if (-not (Test-Path $ModulePath)) { throw "Missing PawnIO module: $ModulePath" }
    $h = [PawnIOCtl]::CreateFileW('\\?\GLOBALROOT\Device\PawnIO',
            [PawnIOCtl]::GENERIC_READ_WRITE, [PawnIOCtl]::FILE_SHARE_RW,
            [IntPtr]::Zero, [PawnIOCtl]::OPEN_EXISTING,
            [PawnIOCtl]::FILE_ATTRIBUTE_NORMAL, [IntPtr]::Zero)
    if ($h -eq [IntPtr]::Zero -or $h.ToInt64() -eq -1) {
        $e = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw ("Cannot open PawnIO device (Win32 {0}). Run elevated and ensure PawnIO service is running." -f $e)
    }
    $bin = [IO.File]::ReadAllBytes($ModulePath)
    $ret = [uint32]0
    if (-not [PawnIOCtl]::DeviceIoControl($h, [PawnIOCtl]::IOCTL_LOAD_BINARY, $bin, [uint32]$bin.Length,
            $null, 0, [ref]$ret, [IntPtr]::Zero)) {
        $e = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        [void][PawnIOCtl]::CloseHandle($h)
        throw ("Failed to load PawnIO module (Win32 {0})" -f $e)
    }
    $script:H = $h
}

function Invoke-PawnIO {
    param([string]$Name, [long[]]$InData, [int]$OutLen)
    $in = New-Object byte[] ([PawnIOCtl]::FN_NAME_LENGTH + $InData.Length * 8)
    $nb = [Text.Encoding]::ASCII.GetBytes($Name)
    [Array]::Copy($nb, 0, $in, 0, [Math]::Min([PawnIOCtl]::FN_NAME_LENGTH - 1, $nb.Length))
    if ($InData.Length -gt 0) { [Buffer]::BlockCopy($InData, 0, $in, [PawnIOCtl]::FN_NAME_LENGTH, $InData.Length * 8) }
    $out = New-Object byte[] ([Math]::Max($OutLen * 8, 8))
    $read = [uint32]0
    if (-not [PawnIOCtl]::DeviceIoControl($script:H, [PawnIOCtl]::IOCTL_EXECUTE,
            $in, [uint32]$in.Length, $out, [uint32]$out.Length, [ref]$read, [IntPtr]::Zero)) {
        $e = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw ("PawnIO execute {0} failed (Win32 {1})" -f $Name, $e)
    }
    $res = New-Object long[] $OutLen
    $n = [int]($read / 8)
    for ($i = 0; $i -lt [Math]::Min($n, $OutLen); $i++) { $res[$i] = [BitConverter]::ToInt64($out, $i * 8) }
    return $res
}

$EC_CMD_PORT  = 0x66
$EC_DATA_PORT = 0x62
$EC_CMD_READ  = 0x80
$EC_CMD_WRITE = 0x81
$EC_OBF       = 0x01
$EC_IBF       = 0x02

function Wait-Tick { [System.Threading.Thread]::SpinWait(100) }

function Read-IOPort {
    param([int]$Port)
    $r = Invoke-PawnIO -Name 'ioctl_pio_read' -InData @([long]$Port) -OutLen 1
    return [byte]($r[0] -band 0xFF)
}

function Wait-IBFClear {
    for ($i = 0; $i -lt 2000; $i++) {
        if ((Read-IOPort $EC_CMD_PORT) -band $EC_IBF) { Wait-Tick } else { return $true }
    }
    return $false
}

function Wait-ReadReady {
    if ($script:ObfUsed -ne $false) {
        for ($i = 0; $i -lt 5; $i++) {
            if ((Read-IOPort $EC_CMD_PORT) -band $EC_OBF) { $script:ObfUsed = $true; return $true }
            Wait-Tick
        }
        $script:ObfUsed = $false
    }
    for ($i = 0; $i -lt 2000; $i++) {
        if (-not ((Read-IOPort $EC_CMD_PORT) -band $EC_IBF)) { return $true }
        Wait-Tick
    }
    return $false
}

function Read-ECByte {
    param([int]$Register)
    for ($attempt = 0; $attempt -lt 5; $attempt++) {
        $s0 = Read-IOPort $EC_CMD_PORT
        if ($s0 -band $EC_OBF) { $null = Read-IOPort $EC_DATA_PORT; Wait-Tick; $s0 = Read-IOPort $EC_CMD_PORT }
        if ($s0 -band $EC_IBF) { Wait-Tick; continue }
        $null = Invoke-PawnIO -Name 'ioctl_pio_write' -InData @([long]$EC_CMD_PORT, [long]$EC_CMD_READ) -OutLen 0
        if ((Read-IOPort $EC_CMD_PORT) -band $EC_IBF) { Wait-Tick; continue }
        $null = Invoke-PawnIO -Name 'ioctl_pio_write' -InData @([long]$EC_DATA_PORT, [long]$Register) -OutLen 0
        if (Wait-ReadReady) { return (Read-IOPort $EC_DATA_PORT) }
        Wait-Tick
    }
    throw ("Read EC 0x{0:X2} failed" -f $Register)
}

function Write-ECByte {
    param([int]$Register, [int]$Val)
    for ($attempt = 0; $attempt -lt 5; $attempt++) {
        if (-not (Wait-IBFClear)) { continue }
        $null = Invoke-PawnIO -Name 'ioctl_pio_write' -InData @([long]$EC_CMD_PORT, [long]$EC_CMD_WRITE) -OutLen 0
        if (-not (Wait-IBFClear)) { continue }
        $null = Invoke-PawnIO -Name 'ioctl_pio_write' -InData @([long]$EC_DATA_PORT, [long]$Register) -OutLen 0
        if (-not (Wait-IBFClear)) { continue }
        $null = Invoke-PawnIO -Name 'ioctl_pio_write' -InData @([long]$EC_DATA_PORT, [long]$Val) -OutLen 0
        return $true
    }
    return $false
}

function Snapshot {
    param([string]$Tag)
    $parts = @()
    foreach ($r in @(0x2C, 0x2D, 0x2E, 0x2F, 0x58, 0x59)) {
        try {
            $v = Read-ECByte $r
            $parts += ('0x{0:X2}=0x{1:X2}({1})' -f $r, $v)
        } catch {
            $parts += ('0x{0:X2}=ERR' -f $r)
        }
    }
    $line = "$Tag " + ($parts -join ' ')
    Write-Host $line
    Add-Content -Path $OutFile -Value $line -Encoding UTF8
    return $line
}

$mtx = $null
$restored = $false
try {
    if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
            [Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw "This script must run elevated."
    }
    $svc = Get-Service PawnIO -ErrorAction SilentlyContinue
    if (-not $svc) { throw "PawnIO service is not installed." }
    if ($svc.Status -ne 'Running') { Start-Service PawnIO; Start-Sleep -Seconds 1 }

    Open-PawnIO -ModulePath $Module
    $mtx = New-Object System.Threading.Mutex($false, 'Global\Access_EC')
    if (-not $mtx.WaitOne(10000)) { throw "Could not acquire Global\Access_EC." }

    Snapshot 'before'
    Write-Host "Writing 0x2C/0x2D = 0x64 (100%)..."
    $ok1 = Write-ECByte 0x2C 0x64
    $ok2 = Write-ECByte 0x2D 0x64
    Write-Host "write ok: 0x2C=$ok1 0x2D=$ok2"
    Start-Sleep -Milliseconds 300
    Snapshot 'after-write'
    Start-Sleep -Seconds 3
    Snapshot 'after-3s'
} catch {
    Write-Host ("ERROR: " + $_.Exception.Message)
    Add-Content -Path $OutFile -Value ("ERROR: " + $_.Exception.Message) -Encoding UTF8
} finally {
    try {
        if ($script:H -ne [IntPtr]::Zero) {
            Write-Host "Restoring 0x2C/0x2D = 0xFF..."
            $r1 = Write-ECByte 0x2C 0xFF
            $r2 = Write-ECByte 0x2D 0xFF
            Write-Host "restore ok: 0x2C=$r1 0x2D=$r2"
            Start-Sleep -Milliseconds 300
            Snapshot 'after-restore'
            $restored = $true
        }
    } catch {
        Write-Host ("RESTORE ERROR: " + $_.Exception.Message)
        Add-Content -Path $OutFile -Value ("RESTORE ERROR: " + $_.Exception.Message) -Encoding UTF8
    }
    if ($mtx) { try { $mtx.ReleaseMutex() } catch {}; try { $mtx.Dispose() } catch {} }
    if ($script:H -ne [IntPtr]::Zero) { [void][PawnIOCtl]::CloseHandle($script:H) }
    Write-Host ("RESTORED=" + $restored)
    Stop-Transcript | Out-Null
}
