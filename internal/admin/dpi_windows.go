//go:build windows

package admin

import "syscall"

var shcore = syscall.NewLazyDLL("shcore.dll")
var procSetProcessDpiAwareness = shcore.NewProc("SetProcessDpiAwareness")

func SetDPIAwareness() {
	if err := shcore.Load(); err == nil {
		ret, _, _ := procSetProcessDpiAwareness.Call(2)
		if ret != 0 {
			procSetProcessDpiAwareness.Call(1)
		}
	}
}
