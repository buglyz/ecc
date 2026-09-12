//go:build windows

package ui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// kit 封装主题与基础绘制工具, 供各控件与页面复用。
type kit struct {
	th *material.Theme
}

type ButtonStyle int

const (
	BtnPrimary ButtonStyle = iota
	BtnSecondary
	BtnOutline
	BtnDanger
	BtnGreenOutline
)

// ---------- 基础绘制 ----------

func fillRect(gtx layout.Context, col color.NRGBA) {
	paint.FillShape(gtx.Ops, col, clip.Rect{Max: gtx.Constraints.Max}.Op())
}

// addClickArea 在当前约束范围内注册点击区域。
// 必须先用 clip 限定范围, 否则区域无界, 一次点击会命中窗口内所有控件。
func addClickArea(gtx layout.Context, c *gesture.Click) {
	stack := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	defer stack.Pop()
	c.Add(gtx.Ops)
}

// paintFillIn 在指定局部矩形内填充纯色。
func paintFillIn(gtx layout.Context, r image.Rectangle, col color.NRGBA) {
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: r.Min, Max: r.Max}.Op())
}

// drawRoundRect 以当前约束区域绘制圆角矩形, 可选边框。
func drawRoundRect(gtx layout.Context, radius unit.Dp, fill, border color.NRGBA, borderWidth unit.Dp) {
	rect := image.Rectangle{Max: gtx.Constraints.Max}
	rr := clip.UniformRRect(rect, gtx.Dp(radius))
	paint.FillShape(gtx.Ops, fill, rr.Op(gtx.Ops))
	if borderWidth > 0 {
		stroke := clip.Stroke{Path: rr.Path(gtx.Ops), Width: float32(gtx.Dp(borderWidth))}
		paint.FillShape(gtx.Ops, border, stroke.Op())
	}
}

func drawRoundRectIn(gtx layout.Context, r image.Rectangle, radius unit.Dp, fill, border color.NRGBA, borderWidth unit.Dp) {
	rr := clip.UniformRRect(r, gtx.Dp(radius))
	paint.FillShape(gtx.Ops, fill, rr.Op(gtx.Ops))
	if borderWidth > 0 {
		stroke := clip.Stroke{Path: rr.Path(gtx.Ops), Width: float32(gtx.Dp(borderWidth))}
		paint.FillShape(gtx.Ops, border, stroke.Op())
	}
}

func drawCircle(gtx layout.Context, center f32.Point, radius float32, fill, border color.NRGBA, borderWidth float32) {
	c := circleAt(center, radius)
	paint.FillShape(gtx.Ops, fill, c.Op(gtx.Ops))
	if borderWidth > 0 {
		stroke := clip.Stroke{Path: c.Path(gtx.Ops), Width: borderWidth}
		paint.FillShape(gtx.Ops, border, stroke.Op())
	}
}

// circleAt 构造以 center 为圆心的 clip.Ellipse。
func circleAt(center f32.Point, radius float32) clip.Ellipse {
	return clip.Ellipse(image.Rectangle{
		Min: image.Pt(int(center.X-radius), int(center.Y-radius)),
		Max: image.Pt(int(center.X+radius), int(center.Y+radius)),
	})
}

// drawLine 绘制线段(相对当前变换, 坐标为局部像素)。
func drawLine(gtx layout.Context, x1, y1, x2, y2 float32, col color.NRGBA, width float32) {
	drawPolyline(gtx, []f32.Point{f32.Pt(x1, y1), f32.Pt(x2, y2)}, col, width)
}

// drawPolyline 绘制折线(相对当前变换, 坐标为局部像素)。
func drawPolyline(gtx layout.Context, pts []f32.Point, col color.NRGBA, width float32) {
	if len(pts) < 2 {
		return
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(pts[0])
	for _, pt := range pts[1:] {
		p.LineTo(pt)
	}
	stroke := clip.Stroke{Path: p.End(), Width: width}
	paint.FillShape(gtx.Ops, col, stroke.Op())
}

// ---------- 文本 ----------

type textStyle struct {
	size      unit.Sp
	color     color.NRGBA
	weight    font.Weight
	alignment text.Alignment
	maxLines  int
}

func (k *kit) text(txt string, st textStyle) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(k.th, st.size, txt)
		l.Color = st.color
		l.Font.Weight = st.weight
		l.Font.Typeface = font.Typeface(FaceMain)
		l.Alignment = st.alignment
		l.MaxLines = st.maxLines
		return l.Layout(gtx)
	}
}

// 字号规格平移自旧实现: 像素值即 96dpi 下的 sp 值。
func (k *kit) title(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 17, color: c, weight: font.Bold, maxLines: 1})
}

func (k *kit) section(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 15, color: c, weight: font.Bold, maxLines: 1})
}

func (k *kit) cardTitle(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 14, color: c, weight: font.Medium, maxLines: 1})
}

func (k *kit) body(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 12, color: c, maxLines: 1})
}

func (k *kit) caption(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 10, color: c, maxLines: 1})
}

func (k *kit) valueBig(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 20, color: c, weight: font.Bold, maxLines: 1})
}

func (k *kit) valueMid(t string, c color.NRGBA) layout.Widget {
	return k.text(t, textStyle{size: 15, color: c, weight: font.Bold, maxLines: 1})
}

// ---------- 按钮 ----------

type uiButton struct {
	click gesture.Click
}

func (b *uiButton) layout(gtx layout.Context, k *kit, txt string, style ButtonStyle, onClick func()) layout.Dimensions {
	addClickArea(gtx, &b.click)
	pointer.CursorPointer.Add(gtx.Ops)
	hover := b.click.Hovered()

	var fill, border, textColor color.NRGBA
	switch style {
	case BtnPrimary:
		fill, border, textColor = ColorAccentGreen, ColorAccentGreen, ColorOnAccent
		if hover {
			fill, border = ColorAccentHover, ColorAccentHover
		}
	case BtnSecondary:
		fill, border, textColor = ColorBtnSecondary, ColorBtnSecondaryB, ColorTextPrimary
		if hover {
			fill = ColorBtnSecondHov
		}
	case BtnOutline:
		fill, border, textColor = ColorControlCard, ColorBtnOutlineBd, ColorTextPrimary
		if hover {
			fill, border = ColorBtnOutlineHov, ColorAccentGreen
		}
	case BtnGreenOutline:
		fill, border, textColor = ColorControlCard, ColorAccentGreen, ColorAccentGreen
		if hover {
			fill = ColorGreenOutHov
		}
	case BtnDanger:
		fill, border, textColor = ColorDangerBg, ColorDangerRed, ColorDangerRed
		if hover {
			fill, textColor = ColorDangerRed, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
		}
	}

	drawRoundRect(gtx, ControlRadius, fill, border, 1)
	layout.Center.Layout(gtx, k.cardTitle(txt, textColor))

	for {
		ev, ok := b.click.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind == gesture.KindClick && onClick != nil {
			onClick()
		}
	}
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// ---------- 开关 ----------

type uiSwitch struct {
	click gesture.Click
}

// layout 绘制滑动开关并处理点击; checked 为当前显示状态, 点击后回调 onChange(新状态)。
func (s *uiSwitch) layout(gtx layout.Context, k *kit, checked bool, onChange func(bool)) layout.Dimensions {
	addClickArea(gtx, &s.click)
	pointer.CursorPointer.Add(gtx.Ops)

	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	bg, knob := ColorSwitchOffBg, ColorSwitchOffKnob
	if checked {
		bg, knob = ColorAccentGreen, ColorSwitchOnKnob
	}
	drawRoundRect(gtx, unit.Dp(gtx.Constraints.Max.Y/2), bg, bg, 0)

	knobR := h/2 - 3
	knobCX := knobR + 2
	if checked {
		knobCX = w - knobR - 2
	}
	drawCircle(gtx, f32.Pt(knobCX, h/2), knobR, knob, knob, 0)

	for {
		ev, ok := s.click.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind == gesture.KindClick && onChange != nil {
			onChange(!checked)
		}
	}
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// ---------- 滑块 ----------

type uiSlider struct {
	f widget.Float
}

// layout 绘制 0-100 滑块; onChange 在拖动改变值时回调(仅本地状态, 不落盘)。
func (s *uiSlider) layout(gtx layout.Context, k *kit, value int, disabled bool, onChange func(int)) layout.Dimensions {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)

	trackH := float32(gtx.Dp(6))
	trackY := (h - trackH) / 2
	thumbR := float32(gtx.Dp(9))
	thumbX := w * float32(value) / 100
	if thumbX < 0 {
		thumbX = 0
	}
	if thumbX > w {
		thumbX = w
	}

	trackBg := ColorSliderTrack
	var thumbFill, thumbBorder color.NRGBA
	switch {
	case disabled:
		trackBg = ColorSliderDisTrk
		thumbFill, thumbBorder = ColorSliderDisThm, ColorSliderDisBd
	case s.f.Dragging():
		thumbFill, thumbBorder = ColorAccentHover, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	default:
		thumbFill, thumbBorder = ColorAccentGreen, ColorThumbBorder
	}

	trackR := image.Rectangle{Min: image.Pt(0, int(trackY)), Max: image.Pt(int(w), int(trackY+trackH))}
	paint.FillShape(gtx.Ops, trackBg, clip.UniformRRect(trackR, gtx.Dp(3)).Op(gtx.Ops))
	if !disabled && thumbX > 0 {
		activeR := image.Rectangle{Min: image.Pt(0, int(trackY)), Max: image.Pt(int(thumbX), int(trackY+trackH))}
		paint.FillShape(gtx.Ops, ColorAccentGreen, clip.UniformRRect(activeR, gtx.Dp(3)).Op(gtx.Ops))
	}

	// 交互层: Float 以最小约束作为区域大小, 外扩 10dp 命中边距
	if !disabled {
		changed := s.f.Update(gtx)
		s.f.Layout(gtx, layout.Horizontal, unit.Dp(10))
		if changed {
			v := int(s.f.Value*100 + 0.5)
			if v < 0 {
				v = 0
			}
			if v > 100 {
				v = 100
			}
			if onChange != nil {
				onChange(v)
			}
		}
		// 非拖动时与状态保持同步 (预设应用/外部修改)
		if !s.f.Dragging() {
			s.f.Value = float32(value) / 100
		}
	}

	// 圆球最后画, 盖住轨道
	drawCircle(gtx, f32.Pt(thumbX, h/2), thumbR, thumbFill, thumbBorder, 1)
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

// ---------- 分段选择器 ----------

type SegmentItem struct {
	Key   string
	Label string
	Desc  string
}

type uiSegmented struct {
	clicks []gesture.Click
}

func (s *uiSegmented) layout(gtx layout.Context, k *kit, items []SegmentItem, selected string, onChange func(key string)) layout.Dimensions {
	if len(items) == 0 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if len(s.clicks) < len(items) {
		clicks := make([]gesture.Click, len(items))
		copy(clicks, s.clicks)
		s.clicks = clicks
	}

	drawRoundRect(gtx, ControlRadius, ColorSegOuterBg, ColorSegOuterBd, 1)

	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(items))
		for i := range items {
			i := i
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				w := gtx.Constraints.Max.X
				if i < len(items)-1 {
					w -= gtx.Dp(2)
				}
				gtx.Constraints = layout.Exact(image.Pt(w, gtx.Constraints.Max.Y))

				isSel := items[i].Key == selected
				if isSel {
					drawRoundRect(gtx, ControlRadius-1, ColorAccentGreen, ColorAccentGreen, 0)
				} else if s.clicks[i].Hovered() {
					drawRoundRect(gtx, ControlRadius-1, ColorSegHover, ColorSegHover, 0)
				}
				txtCol := ColorTextSecondary
				if isSel {
					txtCol = ColorOnAccent
				}
				layout.Center.Layout(gtx, k.cardTitle(items[i].Label, txtCol))

				addClickArea(gtx, &s.clicks[i])
				pointer.CursorPointer.Add(gtx.Ops)
				for {
					ev, ok := s.clicks[i].Update(gtx.Source)
					if !ok {
						break
					}
					if ev.Kind == gesture.KindClick && !isSel && onChange != nil {
						onChange(items[i].Key)
					}
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	})
}
