//go:build windows

package admin

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell32DLL     = syscall.NewLazyDLL("shell32.dll")
	pIsUserAnAdmin = shell32DLL.NewProc("IsUserAnAdmin")
	pShellExecuteW = shell32DLL.NewProc("ShellExecuteW")
)

func IsAdmin() bool {
	ret, _, _ := pIsUserAnAdmin.Call()
	return ret != 0
}

func RelaunchAsAdmin() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	var params string
	if len(os.Args) > 1 {
		params = strings.Join(os.Args[1:], " ")
	}

	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	var paramsPtr *uint16
	if params != "" {
		paramsPtr, _ = syscall.UTF16PtrFromString(params)
	}
	dir, _ := os.Getwd()
	dirPtr, _ := syscall.UTF16PtrFromString(dir)

	ret, _, _ := pShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(dirPtr)),
		1, // SW_SHOWNORMAL
	)
	return ret > 32
}

func RequireAdmin(skip bool) bool {
	if skip || IsAdmin() {
		return true
	}
	RelaunchAsAdmin()
	return false
}
