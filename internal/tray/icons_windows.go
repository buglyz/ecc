//go:build windows

package tray

import (
	"math"
	"syscall"
	"unsafe"
)

// iconSpec 描述一个托盘图标的全部渲染输入。
// Update() 用它判断当前显示值与上次渲染是否相同：相同则跳过 GDI 图标重建。
type iconSpec struct {
	text string
	r    uint8
	g    uint8
	b    uint8
}

// iconForTemp 把温度换算成图标文本与配色。nil 温度显示 "--"（灰色）。
func iconForTemp(temp *float64) iconSpec {
	spec := iconSpec{text: "--", r: 200, g: 200, b: 200}
	if temp != nil {
		t := *temp
		spec.text = itoa16(int(math.Round(t)))
		switch {
		case t < 60:
			spec.r, spec.g, spec.b = 80, 220, 100
		case t < 80:
			spec.r, spec.g, spec.b = 255, 200, 50
		default:
			spec.r, spec.g, spec.b = 255, 90, 90
		}
	}
	return spec
}

// itoa16 生成十进制数字串（温度显示只需 0–100 及 "‑‑" 多一个字符），
// 避免 fmt.Sprintf 在每 1 秒一次的图标刷新里反复进格式化路径。
func itoa16(v int) string {
	if v == 0 {
		return "0"
	}
	buf := [16]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func createTempIcon(temp *float64) syscall.Handle {
	spec := iconForTemp(temp)
	return createIcon(renderText16(spec.text, spec.r, spec.g, spec.b))
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

	return createIcon(pixels)
}

// createIcon 把 16x16 BGRA 像素数组封装成 HICON：两次 CreateDIBSection
// （彩色位图 + 全透明掩码），CreateIconIndirect 合成为图标后释放 DIB。
// 失败返回 0，调用方应保留当前图标。
func createIcon(pixels []byte) syscall.Handle {
	const size = 16
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
