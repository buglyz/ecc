package sensors

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/process"
)

const readTimeout = 5 * time.Second
const closeTimeout = 2 * time.Second
const maxBackoff = 30 * time.Second
const backoffMultiplier = 2
const lhmDLLPathEnv = "ECC_LHM_DLL_PATH"

type PowerShellReader struct {
	DLLPath          string
	Logger           *log.Logger
	mu               sync.Mutex
	cmd              *exec.Cmd
	stdin            *bufio.Writer
	stdout           *bufio.Reader
	stdinPipe        anyWriteCloser
	stdoutPipe       io.Closer
	consecutiveFails int
	lastFailTime     time.Time
	backoffUntil     time.Time
}

type anyWriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

type tempPayload struct {
	CPU *float64 `json:"cpu"`
	GPU *float64 `json:"gpu"`
}

func (r *PowerShellReader) Read() controller.Temps {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Logger == nil {
		r.Logger = log.Default()
	}

	// Check if we're in backoff period
	if !r.backoffUntil.IsZero() && time.Now().Before(r.backoffUntil) {
		return controller.Temps{}
	}

	if r.cmd == nil {
		if err := r.startLocked(); err != nil {
			r.Logger.Printf("温度读取助手启动失败: %v", err)
			r.recordFailureLocked()
			return controller.Temps{}
		}
	}
	if _, err := r.stdin.WriteString("read\n"); err != nil {
		r.Logger.Printf("温度读取请求失败: %v", err)
		r.resetLocked()
		r.recordFailureLocked()
		return controller.Temps{}
	}
	if err := r.stdin.Flush(); err != nil {
		r.Logger.Printf("温度读取请求刷新失败: %v", err)
		r.resetLocked()
		r.recordFailureLocked()
		return controller.Temps{}
	}

	stdout := r.stdout
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := stdout.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()

	var line string
	select {
	case line = <-lineCh:
	case err := <-errCh:
		r.Logger.Printf("温度读取响应失败: %v", err)
		r.resetLocked()
		r.recordFailureLocked()
		return controller.Temps{}
	case <-time.After(readTimeout):
		r.Logger.Printf("温度读取响应超时")
		r.resetLocked()
		r.recordFailureLocked()
		return controller.Temps{}
	}
	var payload tempPayload
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		r.Logger.Printf("温度读取响应解析失败: %v", err)
		r.recordFailureLocked()
		return controller.Temps{}
	}

	// Success - reset failure counter
	if r.consecutiveFails > 0 {
		r.Logger.Printf("温度传感器已恢复，连续失败 %d 次后成功", r.consecutiveFails)
	}
	r.consecutiveFails = 0
	r.backoffUntil = time.Time{}

	return controller.Temps{CPU: payload.CPU, GPU: payload.GPU}
}

func (r *PowerShellReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil {
		return nil
	}
	_, _ = r.stdin.WriteString("quit\n")
	_ = r.stdin.Flush()
	return r.stopLocked(false, closeTimeout)
}

func (r *PowerShellReader) startLocked() error {
	cmd := newPowerShellCommand(r.DLLPath)
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	r.cmd = cmd
	r.stdinPipe = stdinPipe
	r.stdoutPipe = stdoutPipe
	r.stdin = bufio.NewWriter(stdinPipe)
	r.stdout = bufio.NewReader(stdoutPipe)
	return nil
}

func newPowerShellCommand(dllPath string) *exec.Cmd {
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy",
		"Bypass",
		"-EncodedCommand",
		encodePowerShellCommand(powerShellScript),
	)
	cmd.Env = append(withoutEnvironmentVariable(os.Environ(), lhmDLLPathEnv), lhmDLLPathEnv+"="+dllPath)
	return cmd
}

func encodePowerShellCommand(script string) string {
	encoded := utf16.Encode([]rune(script))
	data := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func withoutEnvironmentVariable(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (r *PowerShellReader) resetLocked() {
	_ = r.stopLocked(true, closeTimeout)
}

func (r *PowerShellReader) stopLocked(kill bool, timeout time.Duration) error {
	cmd := r.cmd
	if cmd == nil {
		r.clearLocked()
		return nil
	}
	if r.stdinPipe != nil {
		_ = r.stdinPipe.Close()
	}
	if r.stdoutPipe != nil {
		_ = r.stdoutPipe.Close()
	}
	if kill && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		r.clearLocked()
		return err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case err := <-done:
			r.clearLocked()
			if err != nil {
				return errors.Join(errors.New("temperature helper close timeout"), err)
			}
			return errors.New("temperature helper close timeout")
		case <-time.After(time.Second):
			r.clearLocked()
			return errors.New("temperature helper close timeout")
		}
	}
}

func (r *PowerShellReader) clearLocked() {
	r.cmd = nil
	r.stdin = nil
	r.stdout = nil
	r.stdinPipe = nil
	r.stdoutPipe = nil
}

func (r *PowerShellReader) recordFailureLocked() {
	now := time.Now()
	r.consecutiveFails++
	r.lastFailTime = now

	if r.consecutiveFails >= 3 {
		// Calculate exponential backoff: 1s, 2s, 4s, 8s, ..., max 30s
		backoffDuration := time.Second
		for failure := 3; failure < r.consecutiveFails && backoffDuration < maxBackoff; failure++ {
			backoffDuration *= backoffMultiplier
			if backoffDuration > maxBackoff {
				backoffDuration = maxBackoff
			}
		}
		r.backoffUntil = now.Add(backoffDuration)
		r.Logger.Printf("温度传感器连续失败 %d 次，退避 %v 后重试", r.consecutiveFails, backoffDuration)
	}
}

const powerShellScript = `$ErrorActionPreference = "Stop"
$DllPath = $env:ECC_LHM_DLL_PATH
if ([string]::IsNullOrWhiteSpace($DllPath)) {
  throw "missing LibreHardwareMonitor DLL path"
}
Add-Type -Path $DllPath
$computer = [LibreHardwareMonitor.Hardware.Computer]::new()
$computer.IsCpuEnabled = $true
$computer.IsGpuEnabled = $true
$computer.Open()
function Read-Temps($items, $kind) {
  $matched = @($items | Where-Object { $_.HardwareType.ToString().ToLowerInvariant().Contains($kind) })
  if ($matched.Count -eq 0) { return @() }
  foreach ($item in $matched) {
    $item.Update()
    foreach ($sub in @($item.SubHardware)) { $sub.Update() }
    $temps = @($item.Sensors | Where-Object { $_.SensorType -eq [LibreHardwareMonitor.Hardware.SensorType]::Temperature -and $null -ne $_.Value } | ForEach-Object { [double]$_.Value })
    if ($temps.Count -gt 0) { return $temps }
  }
  return @()
}
try {
  while (($line = [Console]::In.ReadLine()) -ne $null) {
    if ($line -eq "quit") { break }
    try {
      $hardware = @($computer.Hardware)
      $cpuTemps = Read-Temps $hardware "cpu"
      $gpuTemps = Read-Temps $hardware "gpu"
      $cpu = $null
      $gpu = $null
      if ($cpuTemps.Count -ge 2) { $cpu = $cpuTemps[$cpuTemps.Count - 2] } elseif ($cpuTemps.Count -eq 1) { $cpu = $cpuTemps[0] }
      if ($gpuTemps.Count -gt 0) { $gpu = $gpuTemps[0] }
      @{cpu=$cpu; gpu=$gpu} | ConvertTo-Json -Compress
    } catch {
      '{"cpu":null,"gpu":null}'
    }
  }
} finally {
  $computer.Close()
}
`
