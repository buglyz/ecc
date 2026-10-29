//go:build windows

// Package nl5 只负责读取/写入原厂 ControlCenter 的少量状态值。
// 用途是避免 ECC 与原厂 daemon 同时写 EC。
package nl5

import (
	"syscall"
	"unsafe"
)

const (
	KeyPath = `SOFTWARE\WOW6432Node\NL5\ControlCenter`

	ValueFanControlStatus = "FanControlStatus"

	hkeyLocalMachine = 0x80000002
	keyRead          = 0x20019
	keySetValue      = 0x0002
	regDWORD         = 4
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

func openKey(access uint32) (syscall.Handle, error) {
	path, err := syscall.UTF16PtrFromString(KeyPath)
	if err != nil {
		return 0, err
	}
	var h syscall.Handle
	r, _, e := procRegOpenKeyExW.Call(
		uintptr(hkeyLocalMachine),
		uintptr(unsafe.Pointer(path)),
		0,
		uintptr(access),
		uintptr(unsafe.Pointer(&h)),
	)
	if r != 0 {
		return 0, e
	}
	return h, nil
}

func FanControlStatus() (uint32, bool) {
	h, err := openKey(keyRead)
	if err != nil {
		return 0, false
	}
	defer procRegCloseKey.Call(uintptr(h))

	name, err := syscall.UTF16PtrFromString(ValueFanControlStatus)
	if err != nil {
		return 0, false
	}
	var typ, size uint32 = 0, 4
	var value uint32
	r, _, _ := procRegQueryValueExW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(name)),
		0,
		uintptr(unsafe.Pointer(&typ)),
		uintptr(unsafe.Pointer(&value)),
		uintptr(unsafe.Pointer(&size)),
	)
	if r != 0 || typ != regDWORD || size != 4 {
		return 0, false
	}
	return value, true
}

func SetFanControlStatus(value uint32) bool {
	h, err := openKey(keyRead | keySetValue)
	if err != nil {
		return false
	}
	defer procRegCloseKey.Call(uintptr(h))

	name, err := syscall.UTF16PtrFromString(ValueFanControlStatus)
	if err != nil {
		return false
	}
	v := value
	r, _, _ := procRegSetValueExW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(name)),
		0,
		uintptr(regDWORD),
		uintptr(unsafe.Pointer(&v)),
		4,
	)
	return r == 0
}
