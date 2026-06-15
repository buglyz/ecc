//go:build windows

package tray

import (
	"fmt"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

const (
	wmApp           = 0x8000
	wmTrayMsg       = wmApp + 1
	wmCommand       = 0x0111
	wmDestroy       = 0x0002
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idShow = 1001
	idExit = 1002

	mfString     = 0x00000000
	tpmLeftAlign = 0x0000

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	wsExToolWindow = 0x00000080
	wsPopup        = 0x80000000

	colorWindow = 5

	imageIcon = 1
	lrShared  = 0x00008000

	idiApplication = 32512
)

var (
	shell32  = syscall.NewLazyDLL("shell32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")

	pShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	pRegisterClassEx     = user32.NewProc("RegisterClassExW")
	pCreateWindowEx      = user32.NewProc("CreateWindowExW")
	pDefWindowProc       = user32.NewProc("DefWindowProcW")
	pGetMessage          = user32.NewProc("GetMessageW")
	pTranslateMessage    = user32.NewProc("TranslateMessage")
	pDispatchMessage     = user32.NewProc("DispatchMessageW")
	pPostQuitMessage     = user32.NewProc("PostQuitMessage")
	pCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	pAppendMenu          = user32.NewProc("AppendMenuW")
	pTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	pDestroyMenu         = user32.NewProc("DestroyMenu")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	pGetCursorPos        = user32.NewProc("GetCursorPos")
	pPostMessage         = user32.NewProc("PostMessageW")
	pLoadImage           = user32.NewProc("LoadImageW")
	pGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	pCreateIconIndirect  = user32.NewProc("CreateIconIndirect")
	pDestroyIcon         = user32.NewProc("DestroyIcon")
	pCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	pDeleteObject        = gdi32.NewProc("DeleteObject")
)

type point struct{ x, y int32 }

type msg struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   syscall.Handle
	icon       syscall.Handle
	cursor     syscall.Handle
	background syscall.Handle
	menuName   *uint16
	className  *uint16
	iconSm     syscall.Handle
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             syscall.Handle
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            syscall.Handle
	szTip            [128]uint16
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  syscall.Handle
	hbmColor syscall.Handle
}

type Tray struct {
	hwnd        syscall.Handle
	nid         notifyIconData
	defaultIcon syscall.Handle
	onShow      func()
	onExit      func()
	mu          sync.Mutex
	running     bool
	stopCh      chan struct{}
}

var instance *Tray
var instanceMu sync.Mutex

func New(onShow, onExit func()) *Tray {
	t := &Tray{onShow: onShow, onExit: onExit, stopCh: make(chan struct{})}
	instanceMu.Lock()
	instance = t
	instanceMu.Unlock()
	return t
}

func (t *Tray) Run() {
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()

	hInst, _, _ := pGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString("FanControllerTray")

	wc := wndClassEx{
		size:       uint32(unsafe.Sizeof(wndClassEx{})),
		style:      csHRedraw | csVRedraw,
		wndProc:    syscall.NewCallback(wndProc),
		instance:   syscall.Handle(hInst),
		background: syscall.Handle(colorWindow + 1),
		className:  className,
	}
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	windowName, _ := syscall.UTF16PtrFromString("FanControllerTrayWindow")
	hwnd, _, _ := pCreateWindowEx.Call(
		wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		wsPopup, 0, 0, 0, 0, 0, 0,
		hInst, 0,
	)
	t.hwnd = syscall.Handle(hwnd)

	defaultIcon, _, _ := pLoadImage.Call(0, uintptr(idiApplication), imageIcon, 16, 16, lrShared)
	t.defaultIcon = syscall.Handle(defaultIcon)

	t.nid = notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             t.hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayMsg,
		hIcon:            syscall.Handle(defaultIcon),
	}
	copy(t.nid.szTip[:], utf16("风扇控制器"))
	pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))

	var m msg
	for {
		ret, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 || int32(ret) == -1 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}

	pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
	t.mu.Lock()
	if t.nid.hIcon != 0 && t.nid.hIcon != t.defaultIcon {
		pDestroyIcon.Call(uintptr(t.nid.hIcon))
	}
	t.running = false
	t.mu.Unlock()
	close(t.stopCh)
}

func (t *Tray) Update(temp *float64, speed *int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}

	tooltip := "风扇控制器"
	if temp != nil && speed != nil {
		tooltip = fmt.Sprintf("风扇控制器: %.0f°C / %d%%", *temp, *speed)
	}
	for i := range t.nid.szTip {
		t.nid.szTip[i] = 0
	}
	copy(t.nid.szTip[:], utf16(tooltip))

	newIcon := createTempIcon(temp)
	if newIcon != 0 {
		oldIcon := t.nid.hIcon
		t.nid.hIcon = newIcon
		if oldIcon != 0 && oldIcon != t.defaultIcon {
			pDestroyIcon.Call(uintptr(oldIcon))
		}
	}

	t.nid.uFlags = nifMessage | nifIcon | nifTip
	pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
}

func (t *Tray) Alert() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.running {
		return
	}

	// Flash icon red to indicate write failure
	redIcon := createColorIcon(255, 0, 0)
	if redIcon != 0 {
		oldIcon := t.nid.hIcon
		t.nid.hIcon = redIcon
		t.nid.uFlags = nifIcon
		pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))

		// Restore after brief delay (handled by next Update call)
		if oldIcon != 0 && oldIcon != t.defaultIcon && oldIcon != redIcon {
			pDestroyIcon.Call(uintptr(oldIcon))
		}
	}
}

func (t *Tray) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	hwnd := t.hwnd
	t.mu.Unlock()
	pPostMessage.Call(uintptr(hwnd), wmDestroy, 0, 0)
	<-t.stopCh
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmTrayMsg:
		switch lParam {
		case wmRButtonUp:
			showContextMenu(syscall.Handle(hwnd))
		case wmLButtonDblClk:
			instanceMu.Lock()
			cb := instance
			instanceMu.Unlock()
			if cb != nil && cb.onShow != nil {
				go cb.onShow()
			}
		}
		return 0
	case wmCommand:
		switch int(wParam & 0xFFFF) {
		case idShow:
			instanceMu.Lock()
			cb := instance
			instanceMu.Unlock()
			if cb != nil && cb.onShow != nil {
				go cb.onShow()
			}
		case idExit:
			instanceMu.Lock()
			cb := instance
			instanceMu.Unlock()
			if cb != nil && cb.onExit != nil {
				go cb.onExit()
			}
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := pDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func showContextMenu(hwnd syscall.Handle) {
	menu, _, _ := pCreatePopupMenu.Call()
	showText, _ := syscall.UTF16PtrFromString("打开控制台")
	exitText, _ := syscall.UTF16PtrFromString("退出")
	pAppendMenu.Call(menu, mfString, idShow, uintptr(unsafe.Pointer(showText)))
	pAppendMenu.Call(menu, mfString, idExit, uintptr(unsafe.Pointer(exitText)))

	pSetForegroundWindow.Call(uintptr(hwnd))
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pTrackPopupMenu.Call(menu, tpmLeftAlign, uintptr(pt.x), uintptr(pt.y), 0, uintptr(hwnd), 0)
	pDestroyMenu.Call(menu)
}

func createTempIcon(temp *float64) syscall.Handle {
	const size = 16
	text := "--"
	var r, g, b uint8 = 200, 200, 200
	if temp != nil {
		t := *temp
		text = fmt.Sprintf("%d", int(math.Round(t)))
		switch {
		case t < 60:
			r, g, b = 80, 220, 100
		case t < 80:
			r, g, b = 255, 200, 50
		default:
			r, g, b = 255, 90, 90
		}
	}

	pixels := renderText16(text, r, g, b)

	bi := bitmapInfo{
		bmiHeader: bitmapInfoHeader{
			biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:    int32(size),
			biHeight:   -int32(size),
			biPlanes:   1,
			biBitCount: 32,
		},
	}

	var bits unsafe.Pointer
	hBitmap, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hBitmap == 0 || bits == nil {
		if hBitmap != 0 {
			pDeleteObject.Call(hBitmap)
		}
		return 0
	}
	dst := unsafe.Slice((*byte)(bits), size*size*4)
	copy(dst, pixels)

	var maskPtr unsafe.Pointer
	hMask, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&maskPtr)), 0, 0)
	if hMask == 0 || maskPtr == nil {
		if hMask != 0 {
			pDeleteObject.Call(hMask)
		}
		pDeleteObject.Call(hBitmap)
		return 0
	}
	maskDst := unsafe.Slice((*byte)(maskPtr), size*size*4)
	for i := range maskDst {
		maskDst[i] = 0
	}

	ii := iconInfo{
		fIcon:    1,
		hbmMask:  syscall.Handle(hMask),
		hbmColor: syscall.Handle(hBitmap),
	}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	pDeleteObject.Call(hBitmap)
	pDeleteObject.Call(hMask)

	return syscall.Handle(icon)
}

func createColorIcon(r, g, b uint8) syscall.Handle {
	const size = 16
	pixels := make([]byte, size*size*4)

	// Fill entire icon with solid color
	for i := 0; i < size*size; i++ {
		idx := i * 4
		pixels[idx+0] = b   // Blue
		pixels[idx+1] = g   // Green
		pixels[idx+2] = r   // Red
		pixels[idx+3] = 255 // Alpha
	}

	bi := bitmapInfo{
		bmiHeader: bitmapInfoHeader{
			biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:    int32(size),
			biHeight:   -int32(size),
			biPlanes:   1,
			biBitCount: 32,
		},
	}

	var bits unsafe.Pointer
	hBitmap, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hBitmap == 0 || bits == nil {
		if hBitmap != 0 {
			pDeleteObject.Call(hBitmap)
		}
		return 0
	}
	dst := unsafe.Slice((*byte)(bits), size*size*4)
	copy(dst, pixels)

	var maskPtr unsafe.Pointer
	hMask, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&maskPtr)), 0, 0)
	if hMask == 0 || maskPtr == nil {
		if hMask != 0 {
			pDeleteObject.Call(hMask)
		}
		pDeleteObject.Call(hBitmap)
		return 0
	}
	maskDst := unsafe.Slice((*byte)(maskPtr), size*size*4)
	for i := range maskDst {
		maskDst[i] = 0
	}

	ii := iconInfo{
		fIcon:    1,
		hbmMask:  syscall.Handle(hMask),
		hbmColor: syscall.Handle(hBitmap),
	}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))

	pDeleteObject.Call(hBitmap)
	pDeleteObject.Call(hMask)

	return syscall.Handle(icon)
}

func utf16(s string) []uint16 {
	r, _ := syscall.UTF16FromString(s)
	return r
}
