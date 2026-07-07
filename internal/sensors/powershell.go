package sensors

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/process"
)

const readTimeout = 5 * time.Second
const closeTimeout = 2 * time.Second

// 温度助手（powershell.exe + LibreHardwareMonitor）启动失败或中途退出后的重启退避。
// startLocked 只调 cmd.Start()，无法保证子进程存活：若 LHM DLL 损坏或 Add-Type 抛异常
// 导致脚本一启动就退出，Read() 每秒都会走失败路径并重新 spawn 一个 powershell.exe，
// 持续故障时便是每秒一个进程、一天数万次的进程风暴。退避窗内的 Read 直接返回空温度、
// 不再 spawn，间隔从 startBackoffMin 起指数翻倍、封顶 startBackoffMax，成功读到数据即清零。
const (
	startBackoffMin = 2 * time.Second
	startBackoffMax = 60 * time.Second
)

type PowerShellReader struct {
	DLLPath    string
	StateDir   string
	Logger     *log.Logger
	mu         sync.Mutex
	cmd        *exec.Cmd
	stdin      *bufio.Writer
	stdout     *bufio.Reader
	stdinPipe  anyWriteCloser
	stdoutPipe io.Closer
	backoff    time.Duration
	nextStart  time.Time
	now        func() time.Time
}

// clearBackoff 在成功读到温度后清零退避，使下次故障从最小间隔重新开始。
func (r *PowerShellReader) clearBackoff() {
	r.backoff = 0
	r.nextStart = time.Time{}
}

type anyWriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

type tempPayload struct {
	CPU *float64 `json:"cpu"`
	GPU *float64 `json:"gpu"`
}

// nowFn 返回当前时间，测试可注入伪时钟以验证退避窗口而不依赖真实时间。
func (r *PowerShellReader) nowFn() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// noteStartFailureLocked 在子进程启动失败后推进退避窗：间隔从 startBackoffMin
// 起指数翻倍、封顶 startBackoffMax，并据此设定下一次允许 spawn 的时间点。
func (r *PowerShellReader) noteStartFailureLocked() {
	if r.backoff == 0 {
		r.backoff = startBackoffMin
	} else {
		r.backoff *= 2
		if r.backoff > startBackoffMax {
			r.backoff = startBackoffMax
		}
	}
	r.nextStart = r.nowFn().Add(r.backoff)
}

func (r *PowerShellReader) Read() controller.Temps {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Logger == nil {
		r.Logger = log.Default()
	}
	if r.cmd == nil {
		// 退避窗内不再 spawn，直接返回空温度，避免持续故障时的进程风暴。
		if now := r.nowFn(); !r.nextStart.IsZero() && now.Before(r.nextStart) {
			return controller.Temps{}
		}
		if err := r.startLocked(); err != nil {
			r.noteStartFailureLocked()
			r.Logger.Printf("温度读取助手启动失败: %v (退避 %s)", err, r.backoff)
			return controller.Temps{}
		}
	}
	if _, err := r.stdin.WriteString("read\n"); err != nil {
		r.Logger.Printf("温度读取请求失败: %v", err)
		r.resetLocked()
		return controller.Temps{}
	}
	if err := r.stdin.Flush(); err != nil {
		r.Logger.Printf("温度读取请求刷新失败: %v", err)
		r.resetLocked()
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
		return controller.Temps{}
	case <-time.After(readTimeout):
		r.Logger.Printf("温度读取响应超时")
		r.resetLocked()
		return controller.Temps{}
	}
	var payload tempPayload
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		r.Logger.Printf("温度读取响应解析失败: %v", err)
		return controller.Temps{}
	}
	// 成功读到数据：清零退避，使下次故障从最小间隔重新开始。
	r.clearBackoff()
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
	if err := os.MkdirAll(r.StateDir, 0o755); err != nil {
		return err
	}
	scriptPath := filepath.Join(r.StateDir, "lhm-reader.ps1")
	if err := os.WriteFile(scriptPath, []byte(powerShellScript), 0o600); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-DllPath", r.DLLPath)
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

// resetLocked 在读取失败后杀掉并清理子进程，同时推进退避窗：进程虽已起来但
// 读超时/出错时，若不退避，cmd 归零后下一秒又会立即重启，仍是进程风暴。
func (r *PowerShellReader) resetLocked() {
	_ = r.stopLocked(true, closeTimeout)
	r.noteStartFailureLocked()
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

const powerShellScript = `param([string]$DllPath)
$ErrorActionPreference = "Stop"
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
