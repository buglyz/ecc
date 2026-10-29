//go:build windows

package pawnio

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/buglyz/ecc/internal/process"
)

const (
	pawnDeviceType = 41394 << 16
	ioctlLoadBin   = pawnDeviceType | (0x821 << 2)
	ioctlExecute   = pawnDeviceType | (0x841 << 2)

	fnNameLength = 32

	genericReadWrite    = 0xC0000000
	fileShareRW         = 0x00000003
	openExisting        = 3
	fileAttributeNormal = 0x80

	ecCmdPort  = 0x66
	ecDataPort = 0x62
	ecCmdRead  = 0x80
	ecCmdWrite = 0x81
	ecOBF      = 0x01
	ecIBF      = 0x02

	// LpcACPIEC.bin from PawnIO.Modules 0.2.11 (LGPL-2.1).
	moduleSHA256 = "c38fd116e7aff4d1fdb0a494e296be0a6708e5a22fc72f14587442fb7f8f7906"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCreateFileW         = kernel32.NewProc("CreateFileW")
	procDeviceIoControl     = kernel32.NewProc("DeviceIoControl")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
	procCreateMutexW        = kernel32.NewProc("CreateMutexW")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procReleaseMutex        = kernel32.NewProc("ReleaseMutex")
)

// Device 通过 PawnIO 的 LpcACPIEC 模块访问 ACPI EC 端口 0x62/0x66。
type Device struct {
	handle  syscall.Handle
	mutex   syscall.Handle
	obfUsed *bool
	mu      sync.Mutex
	logger  *log.Logger
}

// Open 打开 PawnIO 设备并加载经过哈希校验的 LpcACPIEC.bin。
func Open(modulePath string, logger *log.Logger) (*Device, error) {
	if logger == nil {
		logger = log.Default()
	}
	module, err := loadVerifiedModule(modulePath)
	if err != nil {
		return nil, err
	}

	h, err := openPawnDevice()
	if err != nil {
		// 服务可能是 Manual/Stopped；尝试启动后重试一次。
		startService(logger)
		time.Sleep(500 * time.Millisecond)
		h, err = openPawnDevice()
		if err != nil {
			return nil, fmt.Errorf("打开 PawnIO 设备失败: %w", err)
		}
	}

	if _, err := deviceIoControl(h, ioctlLoadBin, module, nil); err != nil {
		procCloseHandle.Call(uintptr(h))
		return nil, fmt.Errorf("加载 LpcACPIEC.bin 失败: %w", err)
	}

	mtx, err := acquireECMutex()
	if err != nil {
		procCloseHandle.Call(uintptr(h))
		return nil, err
	}

	return &Device{handle: h, mutex: mtx, logger: logger}, nil
}

func loadVerifiedModule(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 PawnIO 模块失败: %w", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, moduleSHA256) {
		return nil, fmt.Errorf("PawnIO 模块哈希不符: got %s, want %s", got, moduleSHA256)
	}
	return data, nil
}

func openPawnDevice() (syscall.Handle, error) {
	path, err := syscall.UTF16PtrFromString(`\\?\GLOBALROOT\Device\PawnIO`)
	if err != nil {
		return 0, err
	}
	r, _, e := procCreateFileW.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(genericReadWrite),
		uintptr(fileShareRW),
		0,
		uintptr(openExisting),
		uintptr(fileAttributeNormal),
		0,
	)
	if syscall.Handle(r) == syscall.InvalidHandle {
		return 0, e
	}
	return syscall.Handle(r), nil
}

func startService(logger *log.Logger) {
	cmd := exec.Command("sc.exe", "start", "PawnIO")
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Printf("启动 PawnIO 服务失败: %v (%s)", err, strings.TrimSpace(string(out)))
		return
	}
	logger.Print("已启动 PawnIO 服务")
}

func acquireECMutex() (syscall.Handle, error) {
	name, err := syscall.UTF16PtrFromString(`Global\Access_EC`)
	if err != nil {
		return 0, err
	}
	r, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if r == 0 {
		return 0, fmt.Errorf("创建 Access_EC 互斥体失败: %w", e)
	}
	h := syscall.Handle(r)
	wait, _, _ := procWaitForSingleObject.Call(uintptr(h), 10000)
	// WAIT_OBJECT_0 = 0, WAIT_ABANDONED = 0x80
	if wait != 0 && wait != 0x80 {
		procCloseHandle.Call(uintptr(h))
		return 0, fmt.Errorf("等待 Access_EC 互斥体失败: %d", wait)
	}
	return h, nil
}

func deviceIoControl(h syscall.Handle, code uint32, in []byte, out []byte) (uint32, error) {
	var returned uint32
	var inPtr, outPtr uintptr
	if len(in) > 0 {
		inPtr = uintptr(unsafe.Pointer(&in[0]))
	}
	if len(out) > 0 {
		outPtr = uintptr(unsafe.Pointer(&out[0]))
	}
	r, _, e := procDeviceIoControl.Call(
		uintptr(h),
		uintptr(code),
		inPtr,
		uintptr(len(in)),
		outPtr,
		uintptr(len(out)),
		uintptr(unsafe.Pointer(&returned)),
		0,
	)
	if r == 0 {
		if e == syscall.Errno(0) {
			return 0, errors.New("DeviceIoControl failed")
		}
		return 0, e
	}
	return returned, nil
}

func (d *Device) execute(name string, args []int64, outLen int) ([]int64, error) {
	in := make([]byte, fnNameLength+8*len(args))
	n := copy(in, name)
	if n > fnNameLength-1 {
		n = fnNameLength - 1
	}
	_ = n
	for i, a := range args {
		binary.LittleEndian.PutUint64(in[fnNameLength+i*8:], uint64(a))
	}

	bufLen := outLen * 8
	if bufLen < 8 {
		bufLen = 8
	}
	out := make([]byte, bufLen)
	returned, err := deviceIoControl(d.handle, ioctlExecute, in, out)
	if err != nil {
		return nil, fmt.Errorf("PawnIO 执行 %s 失败: %w", name, err)
	}
	count := int(returned / 8)
	if count > outLen {
		count = outLen
	}
	res := make([]int64, outLen)
	for i := 0; i < count; i++ {
		res[i] = int64(binary.LittleEndian.Uint64(out[i*8:]))
	}
	return res, nil
}

func (d *Device) portRead(ctx context.Context, port byte) (byte, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r, err := d.execute("ioctl_pio_read", []int64{int64(port)}, 1)
	if err != nil {
		return 0, err
	}
	return byte(r[0] & 0xFF), nil
}

func (d *Device) portWrite(ctx context.Context, port, val byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := d.execute("ioctl_pio_write", []int64{int64(port), int64(val)}, 0)
	return err
}

func spin() {
	time.Sleep(100 * time.Microsecond)
}

func (d *Device) waitIBFClear(ctx context.Context) bool {
	for i := 0; i < 2000; i++ {
		if err := ctx.Err(); err != nil {
			return false
		}
		st, err := d.portRead(ctx, ecCmdPort)
		if err != nil {
			return false
		}
		if st&ecIBF == 0 {
			return true
		}
		spin()
	}
	return false
}

// waitReadReady 复刻 LibreHardwareMonitor 的回退逻辑：本机 EC 不置 OBF，
// 只等 OBF 会 100% 超时，因此先试 OBF，再退化为“等 IBF 清零”。
func (d *Device) waitReadReady(ctx context.Context) bool {
	if d.obfUsed == nil || *d.obfUsed {
		for i := 0; i < 5; i++ {
			st, err := d.portRead(ctx, ecCmdPort)
			if err != nil {
				return false
			}
			if st&ecOBF != 0 {
				v := true
				d.obfUsed = &v
				return true
			}
			spin()
		}
		v := false
		d.obfUsed = &v
	}
	for i := 0; i < 2000; i++ {
		if err := ctx.Err(); err != nil {
			return false
		}
		st, err := d.portRead(ctx, ecCmdPort)
		if err != nil {
			return false
		}
		if st&ecIBF == 0 {
			return true
		}
		spin()
	}
	return false
}

func (d *Device) ReadEC(ctx context.Context, reg byte) (byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for attempt := 0; attempt < 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		s0, err := d.portRead(ctx, ecCmdPort)
		if err != nil {
			return 0, err
		}
		if s0&ecOBF != 0 {
			_, _ = d.portRead(ctx, ecDataPort)
			spin()
			s0, _ = d.portRead(ctx, ecCmdPort)
		}
		if s0&ecIBF != 0 {
			spin()
			continue
		}
		if err := d.portWrite(ctx, ecCmdPort, ecCmdRead); err != nil {
			return 0, err
		}
		if s1, _ := d.portRead(ctx, ecCmdPort); s1&ecIBF != 0 {
			spin()
			continue
		}
		if err := d.portWrite(ctx, ecDataPort, reg); err != nil {
			return 0, err
		}
		if d.waitReadReady(ctx) {
			return d.portRead(ctx, ecDataPort)
		}
		spin()
	}
	return 0, fmt.Errorf("EC 读取 0x%02X 超时", reg)
}

func (d *Device) WriteEC(ctx context.Context, reg, val byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for attempt := 0; attempt < 5; attempt++ {
		if !d.waitIBFClear(ctx) {
			continue
		}
		if err := d.portWrite(ctx, ecCmdPort, ecCmdWrite); err != nil {
			return err
		}
		if !d.waitIBFClear(ctx) {
			continue
		}
		if err := d.portWrite(ctx, ecDataPort, reg); err != nil {
			return err
		}
		if !d.waitIBFClear(ctx) {
			continue
		}
		if err := d.portWrite(ctx, ecDataPort, val); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("EC 写入 0x%02X=0x%02X 超时", reg, val)
}

func (d *Device) Close() error {
	if d.mutex != 0 {
		procReleaseMutex.Call(uintptr(d.mutex))
		procCloseHandle.Call(uintptr(d.mutex))
		d.mutex = 0
	}
	if d.handle != 0 {
		procCloseHandle.Call(uintptr(d.handle))
		d.handle = 0
	}
	return nil
}
