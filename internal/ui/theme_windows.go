//go:build windows

package ui

import (
	"os"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/text"

	"image/color"
)

// 调色板 (由旧 Win32 GDI 主题平移, 色值保持不变)
var (
	ColorTopbar          = nrgba(0x06, 0x44, 0x7F) // #06447F 深海军蓝
	ColorSidebar         = nrgba(0x1D, 0x1F, 0x21) // #1D1F21 近黑灰
	ColorSidebarSelected = nrgba(0x28, 0x2C, 0x30) // #282C30 侧栏选中
	ColorSidebarHover    = nrgba(0x22, 0x25, 0x28) // #222528 侧栏悬停
	ColorMainBg          = nrgba(0x27, 0x29, 0x2B) // #27292B 主背景深灰
	ColorControlCard     = nrgba(0x06, 0x47, 0x86) // #064786 控制卡片深蓝
	ColorControlBorder   = nrgba(0x0B, 0x5A, 0xA8) // #0B5AA8 控制卡片边框
	ColorCurveCard       = nrgba(0x00, 0x15, 0x26) // #001526 曲线卡片深蓝黑
	ColorCurveBorder     = nrgba(0x00, 0x28, 0x4A) // #00284A
	ColorCardPreset      = nrgba(0x1A, 0x26, 0x30) // #1A2630 预设卡片
	ColorCardHover       = nrgba(0x0A, 0x55, 0x9B) // #0A559B

	ColorTextPrimary   = nrgba(0xE8, 0xED, 0xF2) // #E8EDF2 主文字
	ColorTextSecondary = nrgba(0x9D, 0xA8, 0xB3) // #9DA8B3 次级文字
	ColorTextMuted     = nrgba(0x73, 0x80, 0x8E) // #73808E 弱文字
	ColorAccentGreen   = nrgba(0x00, 0xE8, 0x5A) // #00E85A 主强调绿
	ColorAccentHover   = nrgba(0x33, 0xFF, 0x7A) // #33FF7A 强调绿悬停
	ColorDangerRed     = nrgba(0xE5, 0x39, 0x35) // #E53935 危险红
	ColorWarningYellow = nrgba(0xF5, 0x9E, 0x0B) // #F59E0B 警示黄
	ColorBorderSubtle  = nrgba(0x30, 0x36, 0x3C) // #30363C 分隔线
	ColorCurveGrid     = nrgba(0x0C, 0x26, 0x3E) // #0C263E 曲线网格
	ColorCurveLine     = nrgba(0xA0, 0xAF, 0xBE) // #A0AFBE 曲线折线
	ColorInputBg       = nrgba(0x10, 0x16, 0x1C) // #10161C 输入框底色
	ColorInputBorder   = nrgba(0x28, 0x3C, 0x50) // #283C50 输入框边框
	ColorSliderTrack   = nrgba(0x12, 0x30, 0x48) // #123048 滑块轨道
	ColorToastBg       = nrgba(0x14, 0x19, 0x1E) // #14191E Toast 底色
)

// 旧实现中散落在各绘制函数里的内联色, 统一收编为 token
var (
	ColorOnAccent      = nrgba(0x0A, 0x19, 0x0F) // 绿底上的深色文字 (按钮主色/徽标/分段选中)
	ColorItemCardBg    = nrgba(0x04, 0x34, 0x64) // 状态卡内嵌数据格底色
	ColorItemCardBd    = nrgba(0x0F, 0x4B, 0x87) // 状态卡内嵌数据格边框
	ColorPresetActive  = nrgba(0x14, 0x30, 0x41) // 激活预设卡底色
	ColorBtnSecondary  = nrgba(0x2D, 0x37, 0x44) // 次级按钮底色
	ColorBtnSecondaryB = nrgba(0x41, 0x50, 0x5F) // 次级按钮边框
	ColorBtnSecondHov  = nrgba(0x37, 0x44, 0x55) // 次级按钮悬停
	ColorBtnOutlineBd  = nrgba(0x50, 0x6E, 0x8C) // Outline 按钮边框
	ColorBtnOutlineHov = nrgba(0x0F, 0x55, 0x96) // Outline 悬停底色
	ColorGreenOutHov   = nrgba(0x00, 0x32, 0x19) // GreenOutline 悬停底色
	ColorDangerBg      = nrgba(0x3C, 0x14, 0x19) // 危险按钮底色
	ColorSwitchOffBg   = nrgba(0x37, 0x41, 0x4B) // 开关未选中底色
	ColorSwitchOffKnob = nrgba(0xBE, 0xC8, 0xD2) // 开关未选中圆球
	ColorSwitchOnKnob  = nrgba(0x0F, 0x19, 0x14) // 开关选中圆球
	ColorSegOuterBg    = nrgba(0x0F, 0x1E, 0x30) // 分段选择器外框底色
	ColorSegOuterBd    = nrgba(0x23, 0x3C, 0x5A) // 分段选择器外框边框
	ColorSegHover      = nrgba(0x19, 0x30, 0x4B) // 分段悬停底色
	ColorToastErrText  = nrgba(0xFF, 0xA0, 0xA0) // Toast 错误文字
	ColorChartBg       = nrgba(0x02, 0x12, 0x20) // 曲线图表底色
	ColorTopbarShadow  = nrgba(0x04, 0x28, 0x4B) // 顶栏底部阴影线
	ColorThumbBorder   = nrgba(0xE6, 0xF5, 0xEB) // 滑块常态圆球边框
	ColorSliderDisTrk  = nrgba(0x23, 0x2D, 0x37) // 滑块禁用轨道
	ColorSliderDisThm  = nrgba(0x50, 0x5A, 0x64) // 滑块禁用圆球
	ColorSliderDisBd   = nrgba(0x64, 0x6E, 0x78) // 滑块禁用圆球边框
)

func nrgba(r, g, b byte) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: 0xFF}
}

// 几何 token (dp), 平移自旧布局常量
const (
	TopbarH        = 48
	SidebarW       = 88
	NavItemH       = 64
	CardRadius     = 8
	ControlRadius  = 6
	SmallRadius    = 4
	ToastRadius    = 8
	CardGap        = 14
	CardH          = 176
	CurveCardH     = 230
	CurveCardHTall = 260
	PresetCardW    = 220
	PresetCardH    = 76
	FabSize        = 42
	ToastMinW      = 220
	ToastTop       = 20
)

const (
	FaceMain = "Segoe UI"
	FaceMono = "Consolas"
)

// buildFontCollection 优先加载系统 Segoe UI / Consolas, 失败时回退 Gio 内置 Go 字体。
// buildFontCollection 显式加载 UI 需要的系统字体:
// Segoe UI (拉丁) + Consolas (等宽) + 微软雅黑 (中文回退)。
// 禁用 fontscan 系统扫描: 它会全量解析系统字体并保留上百 MB 内存,
// 且与 Gio 窗口初始化内部的字体处理并发时有卡死风险。
func buildFontCollection() []font.FontFace {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	fontsDir := filepath.Join(windir, "Fonts")

	type spec struct {
		file   string
		weight font.Weight
		family string
	}
	specs := []spec{
		{"segoeui.ttf", font.Normal, FaceMain},
		{"seguisb.ttf", font.Medium, FaceMain},
		{"segoeuib.ttf", font.Bold, FaceMain},
		{"consola.ttf", font.Normal, FaceMono},
		{"consolab.ttf", font.Bold, FaceMono},
	}

	var faces []font.FontFace
	for _, s := range specs {
		data, err := os.ReadFile(filepath.Join(fontsDir, s.file))
		if err != nil {
			continue
		}
		face, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		faces = append(faces, font.FontFace{
			Font: font.Font{Typeface: font.Typeface(s.family), Weight: s.weight},
			Face: face,
		})
	}

	// 中文回退: 微软雅黑。仅载入常规字重文件 (typesetting 急切解析全部字形,
	// 每字重约 70MB); 同一 Face 补登记为 Medium/Bold 条目, 让粗体中文标题
	// 能精确匹配权重, 否则 shaper 对权重差异过大的回退会命中失败出现豆腐块。
	if data, err := os.ReadFile(filepath.Join(fontsDir, "msyh.ttc")); err == nil {
		if cjk, err := opentype.ParseCollection(data); err == nil && len(cjk) > 0 {
			base := cjk[0]
			for _, w := range []font.Weight{font.Normal, font.Medium, font.Bold} {
				faces = append(faces, font.FontFace{
					Font: font.Font{Typeface: base.Font.Typeface, Weight: w},
					Face: base.Face,
				})
			}
		}
	}

	if len(faces) == 0 {
		return gofont.Collection()
	}
	return faces
}

func newShaper(faces []font.FontFace) *text.Shaper {
	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
}
