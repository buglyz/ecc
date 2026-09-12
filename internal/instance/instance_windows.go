//go:build windows

package instance

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

const mutexName = "Local\\ECCFanController"

var (
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	createMutex = kernel32.NewProc("CreateMutexW")
	closeHandle = kernel32.NewProc("CloseHandle")
)

func Acquire() (release func(), alreadyRunning bool, err error) {
	name, err := syscall.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, false, err
	}
	handle, _, callErr := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return nil, false, fmt.Errorf("创建单实例锁失败: %w", callErr)
	}
	if callErr == syscall.ERROR_ALREADY_EXISTS {
		closeHandle.Call(handle)
		return nil, true, nil
	}

	var once sync.Once
	return func() {
		once.Do(func() { closeHandle.Call(handle) })
	}, false, nil
}
