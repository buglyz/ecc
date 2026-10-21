//go:build windows

package instance

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// DefaultMutexName 使用 Global 命名空间，确保同一台机器上不同登录会话（如 RDP
// 与本地控制台）也只有一个实例操作 EC。管理员进程可以创建/打开该对象。
const DefaultMutexName = `Global\ECC-FanController-SingleInstance`

var (
	kernel32DLL   = syscall.NewLazyDLL("kernel32.dll")
	pCreateMutexW = kernel32DLL.NewProc("CreateMutexW")
)

// Guard 持有单实例命名互斥体。进程退出时 Windows 会自动关闭句柄，Release 只是
// 让语义更明确并允许测试在同一进程内重复获取。
type Guard struct {
	handle syscall.Handle
}

// Acquire 尝试获取命名互斥体。
//   - (guard, true, nil)  当前进程是唯一实例
//   - (nil, false, nil)   已有其他实例持有（或无权打开已有互斥体）
//   - (nil, false, err)   创建失败，调用方应拒绝继续操作硬件
func Acquire(name string) (*Guard, bool, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, err
	}
	handle, _, callErr := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	// ERROR_ACCESS_DENIED 表示互斥体已存在且当前进程无权打开（通常由管理员实例
	// 创建），同样视为“已有实例在运行”。
	if errors.Is(callErr, syscall.ERROR_ALREADY_EXISTS) || errors.Is(callErr, syscall.ERROR_ACCESS_DENIED) {
		if handle != 0 {
			_ = syscall.CloseHandle(syscall.Handle(handle))
		}
		return nil, false, nil
	}
	if handle == 0 {
		return nil, false, fmt.Errorf("CreateMutexW(%q): %w", name, callErr)
	}
	return &Guard{handle: syscall.Handle(handle)}, true, nil
}

func (g *Guard) Release() {
	if g == nil || g.handle == 0 {
		return
	}
	_ = syscall.CloseHandle(g.handle)
	g.handle = 0
}
