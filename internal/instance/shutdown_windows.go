//go:build windows

package instance

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// DefaultShutdownEventName 是运行实例监听的外部退出事件。安装器/卸载器通过
// SignalShutdown 触发它，让进程走正常退出流程（defer fan.Stop() 写 0xFF 释放
// EC 风扇控制），避免像 taskkill /F 那样把 EC 留在最后一个固定转速。
const DefaultShutdownEventName = `Global\ECC-FanController-Shutdown`

const (
	eventModifyState = 0x0002
	infiniteWait     = 0xFFFFFFFF
)

var (
	pCreateEventW        = kernel32DLL.NewProc("CreateEventW")
	pOpenEventW          = kernel32DLL.NewProc("OpenEventW")
	pSetEvent            = kernel32DLL.NewProc("SetEvent")
	pResetEvent          = kernel32DLL.NewProc("ResetEvent")
	pWaitForSingleObject = kernel32DLL.NewProc("WaitForSingleObject")
)

// ShutdownListener 持有手动复位的退出事件，Done() 在收到信号后关闭。
type ShutdownListener struct {
	handle syscall.Handle
	done   chan struct{}
	once   sync.Once
}

func NewShutdownListener(name string) (*ShutdownListener, error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	// bManualReset=TRUE, bInitialState=FALSE。
	handle, _, callErr := pCreateEventW.Call(0, 1, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		return nil, fmt.Errorf("CreateEventW(%q): %w", name, callErr)
	}
	// 事件可能是上一次进程残留且处于已触发状态，启动时先复位，
	// 否则新实例会立刻把自己关掉。
	pResetEvent.Call(handle)

	listener := &ShutdownListener{handle: syscall.Handle(handle), done: make(chan struct{})}
	go func(h syscall.Handle) {
		pWaitForSingleObject.Call(uintptr(h), infiniteWait)
		listener.once.Do(func() { close(listener.done) })
	}(listener.handle)
	return listener, nil
}

func (l *ShutdownListener) Done() <-chan struct{} {
	return l.done
}

func (l *ShutdownListener) Close() {
	if l == nil || l.handle == 0 {
		return
	}
	_ = syscall.CloseHandle(l.handle)
	l.handle = 0
}

// SignalShutdown 通知已运行实例优雅退出。没有实例在运行时返回 nil。
func SignalShutdown(name string) error {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	handle, _, callErr := pOpenEventW.Call(eventModifyState, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		if errors.Is(callErr, syscall.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		return fmt.Errorf("OpenEventW(%q): %w", name, callErr)
	}
	defer syscall.CloseHandle(syscall.Handle(handle))
	if ok, _, setErr := pSetEvent.Call(handle); ok == 0 {
		return fmt.Errorf("SetEvent(%q): %w", name, setErr)
	}
	return nil
}
