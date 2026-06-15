$dllPath = "D:/桌面/ControlCenter/LibreHardwareMonitorLib.dll"
Add-Type -Path $dllPath

$computer = New-Object LibreHardwareMonitor.Hardware.Computer
$computer.IsCpuEnabled = $true
$computer.IsGpuEnabled = $true
$computer.IsMotherboardEnabled = $true
$computer.Open()

Write-Host "扫描硬件风扇传感器..."

foreach ($hw in $computer.Hardware) {
    $hw.Update()
    Write-Host "`n硬件: $($hw.Name) ($($hw.HardwareType))"
    
    foreach ($sensor in $hw.Sensors) {
        if ($sensor.SensorType -eq 'Fan') {
            Write-Host "  风扇: $($sensor.Name) = $($sensor.Value) RPM"
        }
    }
    
    foreach ($subhw in $hw.SubHardware) {
        $subhw.Update()
        Write-Host "  子硬件: $($subhw.Name)"
        foreach ($sensor in $subhw.Sensors) {
            if ($sensor.SensorType -eq 'Fan') {
                Write-Host "    风扇: $($sensor.Name) = $($sensor.Value) RPM"
            }
        }
    }
}

$computer.Close()
