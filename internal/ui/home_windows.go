//go:build windows

package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/buglyz/ecc/internal/app"
	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
)

// 页面与框架渲染。坐标体系为 Gio 的 dp (随 DPI 缩放), 尺寸 token 见 theme_windows.go。

var transparentColor = color.NRGBA{}

// ---------- 图标 (纯矢量线绘, 平移自旧实现) ----------

func fanIcon(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	cx, cy := w/2, h/2
	r := (minF(w, h) / 2) - 2
	lw := float32(gtx.Dp(2))

	for i := 0; i < 3; i++ {
		angle := float64(i) * (2 * math.Pi / 3)
		x1 := cx + float32(float64(r)*0.3*math.Cos(angle))
		y1 := cy + float32(float64(r)*0.3*math.Sin(angle))
		x2 := cx + float32(float64(r)*math.Cos(angle+0.4))
		y2 := cy + float32(float64(r)*math.Sin(angle+0.4))
		drawPolyline(gtx, []f32.Point{f32.Pt(x1, y1), f32.Pt(x2, y2)}, col, lw)
	}
	drawCircle(gtx, f32.Pt(cx, cy), r*0.22, col, col, 0)
	drawCircle(gtx, f32.Pt(cx, cy), r, transparentColor, col, lw)
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func homeIcon(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	cx, cy := w/2, h/2
	lw := float32(gtx.Dp(2))
	drawPolyline(gtx, []f32.Point{
		f32.Pt(cx - 8, cy), f32.Pt(cx, cy - 8), f32.Pt(cx + 8, cy),
	}, col, lw)
	drawPolyline(gtx, []f32.Point{
		f32.Pt(cx - 6, cy - 1), f32.Pt(cx - 6, cy + 8), f32.Pt(cx + 6, cy + 8), f32.Pt(cx + 6, cy - 1),
	}, col, lw)
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func settingsIcon(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	cx, cy := w/2, h/2
	drawCircle(gtx, f32.Pt(cx, cy), 4, transparentColor, col, 2)
	drawCircle(gtx, f32.Pt(cx, cy), 7, transparentColor, col, 1)
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func aboutIcon(gtx layout.Context, col color.NRGBA) layout.Dimensions {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	cx, cy := w/2, h/2
	drawCircle(gtx, f32.Pt(cx, cy), 8, transparentColor, col, 1)
	drawLine(gtx, cx, cy-4, cx, cy-2, col, 2)
	drawLine(gtx, cx, cy, cx, cy+4, col, 2)
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// ---------- 框架: 顶栏 + 侧栏 + 内容 ----------

func (w *Window) layoutShell(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(TopbarH)))
			w.layoutTopbar(gtx, k, st)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(SidebarW), gtx.Constraints.Max.Y))
					w.layoutSidebar(gtx, k)
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return w.layoutContent(gtx, k, st)
				}),
			)
		}),
	)
}

func (w *Window) layoutTopbar(gtx layout.Context, k *kit, st *app.State) {
	fillRect(gtx, ColorTopbar)
	h := float32(gtx.Constraints.Max.Y)
	wPx := float32(gtx.Constraints.Max.X)

	layout.Inset{Left: unit.Dp(16), Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(26), gtx.Dp(26)))
				return fanIcon(gtx, ColorAccentGreen)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return k.title("ECC Fan Control", ColorTextPrimary)(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return k.caption("硬件风扇控制台", ColorTextSecondary)(gtx)
				})
			}),
		)
	})
	drawLine(gtx, 0, h-1, wPx, h-1, ColorTopbarShadow, 1)
}

func (w *Window) layoutSidebar(gtx layout.Context, k *kit) {
	fillRect(gtx, ColorSidebar)
	x := float32(gtx.Constraints.Max.X) - 1
	drawLine(gtx, x, 0, x, float32(gtx.Constraints.Max.Y), ColorBorderSubtle, 1)

	type navDef struct {
		tab   NavTab
		label string
		icon  func(layout.Context, color.NRGBA) layout.Dimensions
	}
	navs := []navDef{
		{TabHome, "控制台", homeIcon},
		{TabSettings, "系统设置", settingsIcon},
		{TabAbout, "关于", aboutIcon},
	}

	layout.Inset{Top: unit.Dp(16), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(navs))
		for i, n := range navs {
			n := n
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if i > 0 {
					gtx.Constraints.Max.Y -= gtx.Dp(8)
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(NavItemH)))
						w.layoutNavItem(gtx, k, n.tab, n.label, n.icon)
						return layout.Dimensions{Size: gtx.Constraints.Min}
					})
				}
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(NavItemH)))
				w.layoutNavItem(gtx, k, n.tab, n.label, n.icon)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) layoutNavItem(gtx layout.Context, k *kit, tab NavTab, label string, icon func(layout.Context, color.NRGBA) layout.Dimensions) {
	click := &w.widgets.navClicks[tab]
	addClickArea(gtx, click)
	pointer.CursorPointer.Add(gtx.Ops)

	selected := w.tab == tab
	if selected {
		drawRoundRect(gtx, ControlRadius, ColorSidebarSelected, ColorSidebarSelected, 0)
		// 最左侧 3dp 亮绿指示竖条
		h := gtx.Constraints.Max.Y
		bar := image.Rectangle{Min: image.Pt(0, gtx.Dp(8)), Max: image.Pt(gtx.Dp(3), h-gtx.Dp(8))}
		paintFillIn(gtx, bar, ColorAccentGreen)
	} else if click.Hovered() {
		drawRoundRect(gtx, ControlRadius, ColorSidebarHover, ColorSidebarHover, 0)
	}

	iconCol, textCol := ColorTextSecondary, ColorTextSecondary
	if selected {
		iconCol, textCol = ColorAccentGreen, ColorTextPrimary
	}

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(24), gtx.Dp(20)))
					return icon(gtx, iconCol)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Center.Layout(gtx, k.caption(label, textCol))
			})
		}),
	)

	for {
		ev, ok := click.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind == gesture.KindClick && w.tab != tab {
			w.tab = tab
			w.needInvalidate = true
		}
	}
}

// ---------- 内容区分发 ----------

func (w *Window) layoutContent(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(16), Right: unit.Dp(16), Bottom: unit.Dp(16), Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		switch w.tab {
		case TabHome:
			return w.layoutHome(gtx, k, st)
		case TabSettings:
			return w.layoutSettings(gtx, k, st)
		default:
			return w.layoutAbout(gtx, k)
		}
	})
}

func sectionTitle(k *kit, txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(24)))
		return layout.W.Layout(gtx, k.section(txt, ColorTextPrimary))
	}
}

// ---------- 首页 (参考实现) ----------

func (w *Window) layoutHome(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	wide := gtx.Constraints.Max.X >= gtx.Dp(800)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(sectionTitle(k, "Controls · 控制台")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if wide {
					return w.layoutControlCardsRow(gtx, k, st)
				}
				return w.layoutControlCardsColumn(gtx, k, st)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(24)}.Layout(gtx, sectionTitle(k, "Curves · 5 点温度/转速曲线"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(CurveCardH)))
				w.layoutCurveCard(gtx, k, st)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.layoutCurvePointsRow(gtx, k, st)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(16)}.Layout(gtx, sectionTitle(k, "Presets · 场景预设"))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.layoutPresetCardsRow(gtx, k, st)
			})
		}),
	)
}

// 卡片外框 (圆角 8 + 边框)
func cardFrame(gtx layout.Context) {
	drawRoundRect(gtx, CardRadius, ColorControlCard, ColorControlBorder, 1)
}

func (w *Window) layoutControlCardsRow(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	cards := []func(layout.Context, *kit){
		func(gtx layout.Context, k *kit) { w.layoutStatusCard(gtx, k, st) },
		func(gtx layout.Context, k *kit) { w.layoutModeCard(gtx, k, st) },
		func(gtx layout.Context, k *kit) { w.layoutStrategyCard(gtx, k, st) },
	}
	gap := gtx.Dp(CardGap)
	children := make([]layout.FlexChild, 0, len(cards))
	for i, c := range cards {
		i, c := i, c
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			width := gtx.Constraints.Max.X
			if i < len(cards)-1 {
				width -= gap
			}
			gtx.Constraints = layout.Exact(image.Pt(width, gtx.Dp(CardH)))
			cardFrame(gtx)
			c(gtx, k)
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (w *Window) layoutControlCardsColumn(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(w.fixedCard(func(gtx layout.Context) { w.layoutStatusCard(gtx, k, st) })),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(CardGap)}.Layout(gtx, w.fixedCard(func(gtx layout.Context) { w.layoutModeCard(gtx, k, st) }))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(CardGap)}.Layout(gtx, w.fixedCard(func(gtx layout.Context) { w.layoutStrategyCard(gtx, k, st) }))
		}),
	)
}

func (w *Window) fixedCard(body func(layout.Context)) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(CardH)))
		cardFrame(gtx)
		body(gtx)
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
}

// --- 状态卡 ---

type statusGridItem struct {
	label, val, sub string
	col             color.NRGBA
}

func (w *Window) layoutStatusCard(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	lat := st.Latest
	timeStr := "--"
	if !lat.UpdatedAt.IsZero() {
		timeStr = lat.UpdatedAt.Format("15:04:05")
	}

	items := []statusGridItem{
		{"CPU 温度", formatTemp(lat.CPU), "核心传感器", tempColor(lat.CPU)},
		{"GPU 温度", formatTemp(lat.GPU), "核心传感器", tempColor(lat.GPU)},
		{"目标温度", formatTemp(lat.TargetTemp), "策略计算值", tempColor(lat.TargetTemp)},
		{"风扇输出", formatSpeed(lat.Speed), formatRPM(lat.ActualRPM), ColorAccentGreen},
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(k.section("实时硬件状态", ColorTextPrimary)),
					layout.Rigid(k.caption("更新: "+timeStr, ColorTextSecondary)),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return w.statusGridRow(gtx, k, items[0], items[1])
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(0, gtx.Dp(8))}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return w.statusGridRow(gtx, k, items[2], items[3])
					}),
				)
			})
		}),
	)
}

func (w *Window) statusGridRow(gtx layout.Context, k *kit, a, b statusGridItem) layout.Dimensions {
	cell := func(it statusGridItem) layout.FlexChild {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			drawRoundRect(gtx, ControlRadius, ColorItemCardBg, ColorItemCardBd, 1)
			return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(k.caption(it.label, ColorTextSecondary)),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, k.valueMid(it.val, it.col))
					}),
					layout.Rigid(k.caption(it.sub, ColorTextMuted)),
				)
			})
		})
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		cell(a),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Dp(12), 0)}
		}),
		cell(b),
	)
}

// --- 调速模式卡 ---

func (w *Window) layoutModeCard(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	manualSpeed := w.manualSpeed(st)
	modeDesc := "自动曲线调速"
	if st.Config.ManualEnabled {
		modeDesc = "手动锁定转速"
	}

	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(k.section("调速模式", ColorTextPrimary)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(44), gtx.Dp(22)))
						return w.widgets.modeSwitch.layout(gtx, k, st.Config.ManualEnabled, func(checked bool) { w.onModeSwitch(st, checked) })
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, k.body(modeDesc, ColorAccentGreen))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, k.caption(fmt.Sprintf("手动设定转速: %d%%", manualSpeed), ColorTextSecondary))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(22)))
					return w.widgets.slider.layout(gtx, k, manualSpeed, !st.Config.ManualEnabled, w.onManualSpeed)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(30)))
				return w.widgets.applyBtn.layout(gtx, k, "应用模式设置", BtnGreenOutline, func() { w.onApplyMode(st) })
			}),
		)
	})
}

// --- 温度策略卡 ---

func strategyDesc(key string) string {
	switch key {
	case "weighted":
		return "综合考量 CPU(70%) 与 GPU(30%) 进行加权平衡"
	case "max":
		return "直接取 CPU 与 GPU 中当前最高温度控制风扇"
	case "cpu":
		return "仅根据 CPU 温度变化调节风扇"
	case "gpu":
		return "仅根据 GPU 独立显卡温度调节风扇"
	}
	return "根据选定传感器计算目标温度作为曲线输入"
}

func (w *Window) layoutStrategyCard(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(k.section("温度策略", ColorTextPrimary)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, k.caption(strategyDesc(st.Config.Strategy), ColorTextSecondary))
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(44)))
				return w.widgets.segmented.layout(gtx, k, w.strategyItems, st.Config.Strategy, func(key string) {
					w.onStrategyChange(st, key)
				})
			}),
		)
	})
}

// --- 曲线卡片 (占位: 完整编辑器为迁移第 1 轮任务) ---

func (w *Window) layoutCurveCard(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	drawRoundRect(gtx, CardRadius, ColorCurveCard, ColorCurveBorder, 1)

	// 图表区域: 平移旧实现内边距 左40/右20/上24/下30
	layout.Inset{Top: unit.Dp(24), Right: unit.Dp(20), Bottom: unit.Dp(30), Left: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		chartW := float32(gtx.Constraints.Max.X)
		chartH := float32(gtx.Constraints.Max.Y)
		fillRect(gtx, ColorChartBg)

		// 横向网格: 转速 0/25/50/75/100%
		for i := 0; i <= 4; i++ {
			y := chartH * float32(i) / 4
			drawLine(gtx, 0, y, chartW, y, ColorCurveGrid, 1)
		}
		// 纵向网格: 温度 30..100 每 10°C
		for i := 0; i <= 7; i++ {
			x := chartW * float32(i) / 7
			drawLine(gtx, x, 0, x, chartH, ColorCurveGrid, 1)
		}

		// 折线与控制点 (只读示意)
		if pts := st.Config.Curve; len(pts) >= 2 {
			line := make([]f32.Point, 0, len(pts))
			for _, p := range pts {
				line = append(line, w.curvePointPos(chartW, chartH, p))
			}
			drawPolyline(gtx, line, ColorCurveLine, 2)
			for _, pos := range line {
				drawCircle(gtx, pos, float32(gtx.Dp(5)), ColorAccentGreen, ColorAccentGreen, 2)
			}
		}
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})

	layout.Center.Layout(gtx, k.caption("曲线编辑器迁移中 · 交互规格见 docs/gio-ui-migration.md", ColorTextMuted))
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (w *Window) curvePointPos(chartW, chartH float32, p controller.Point) f32.Point {
	tx := (p.Temp() - controller.CurveTempMin) / (controller.CurveTempMax - controller.CurveTempMin)
	sy := (p.Speed() - controller.CurveSpeedMin) / (controller.CurveSpeedMax - controller.CurveSpeedMin)
	return f32.Pt(chartW*float32(tx), chartH*(1-float32(sy)))
}

func (w *Window) layoutCurvePointsRow(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	pts := st.Config.Curve
	children := make([]layout.FlexChild, 0, len(pts))
	for i := range pts {
		i := i
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			width := gtx.Constraints.Max.X
			if i < len(pts)-1 {
				width -= gtx.Dp(8)
			}
			gtx.Constraints = layout.Exact(image.Pt(width, gtx.Dp(30)))
			drawRoundRect(gtx, SmallRadius, ColorInputBg, ColorInputBorder, 1)
			layout.Center.Layout(gtx, k.caption(fmt.Sprintf("#%d: %.0f°C → %.0f%%", i+1, pts[i].Temp(), pts[i].Speed()), ColorTextSecondary))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}))
		if i < len(pts)-1 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Dp(8), 0)}
			}))
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

// --- 预设卡 (只读占位: 卡片操作与新建对话框为迁移第 2 轮任务) ---

func (w *Window) layoutPresetCardsRow(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	keys := make([]string, 0, len(st.Config.Presets))
	for key := range st.Config.Presets {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		return k.caption("暂无预设", ColorTextMuted)(gtx)
	}

	gap := gtx.Dp(12)
	children := make([]layout.FlexChild, 0, len(keys))
	for i, key := range keys {
		key := key
		preset := st.Config.Presets[key]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			cardW := gtx.Dp(PresetCardW)
			if i < len(keys)-1 {
				cardW += gap
			}
			gtx.Constraints = layout.Exact(image.Pt(cardW, gtx.Dp(PresetCardH)))
			paintCardPreset(gtx, k, preset, w.isPresetActive(preset))
			return layout.Dimensions{Size: image.Pt(cardW, gtx.Dp(PresetCardH))}
		}))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func paintCardPreset(gtx layout.Context, k *kit, p config.PresetConfig, active bool) {
	bg, border := ColorCardPreset, ColorControlBorder
	if active {
		bg, border = ColorPresetActive, ColorAccentGreen
	}
	drawRoundRect(gtx, CardRadius, bg, border, 1)

	layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(14), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(k.cardTitle(p.Label, ColorTextPrimary)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !active {
							return layout.Dimensions{}
						}
						gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(56), gtx.Dp(18)))
						drawRoundRect(gtx, SmallRadius, ColorAccentGreen, ColorAccentGreen, 0)
						layout.Center.Layout(gtx, k.caption("当前生效", ColorOnAccent))
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
				)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
			layout.Rigid(k.caption("应用 / 保存 / 恢复或删除 — 待迁移", ColorTextMuted)),
		)
	})
}

func (w *Window) isPresetActive(p config.PresetConfig) bool {
	st := w.snapshot.Load()
	if st == nil || st.Config.Strategy != p.Strategy || len(st.Config.Curve) != len(p.Curve) {
		return false
	}
	for i := range st.Config.Curve {
		if st.Config.Curve[i] != p.Curve[i] {
			return false
		}
	}
	return true
}

// --- 设置页 (自启动按钮为迁移第 3 轮任务) ---

func (w *Window) layoutSettings(gtx layout.Context, k *kit, st *app.State) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(28)))
			return layout.W.Layout(gtx, k.title("系统集成与运行设置", ColorTextPrimary))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				cardW := gtx.Dp(500)
				if cardW > gtx.Constraints.Max.X {
					cardW = gtx.Constraints.Max.X
				}
				gtx.Constraints = layout.Exact(image.Pt(cardW, gtx.Dp(180)))
				drawRoundRect(gtx, CardRadius, ColorControlCard, ColorControlBorder, 1)

				status := "当前状态：已禁用"
				if st.Startup {
					status = "当前状态：已启用 (通过任务计划程序免 UAC 启动)"
				}
				return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(16), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(k.section("开机自启动", ColorTextPrimary)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, k.body(status, ColorTextSecondary))
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, k.caption("「切换自启动」按钮 — 待迁移", ColorWarningYellow))
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
						layout.Rigid(k.caption("托盘特性：关闭主窗口将自动最小化到系统右下角托盘，双击托盘图标即可随时唤出控制台。", ColorTextMuted)),
					)
				})
			})
		}),
	)
}

// --- 关于页 ---

func (w *Window) layoutAbout(gtx layout.Context, k *kit) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(28)))
			return layout.W.Layout(gtx, k.title("关于 ECC Fan Controller", ColorTextPrimary))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				cardW := gtx.Dp(540)
				if cardW > gtx.Constraints.Max.X {
					cardW = gtx.Constraints.Max.X
				}
				gtx.Constraints = layout.Exact(image.Pt(cardW, gtx.Dp(200)))
				drawRoundRect(gtx, CardRadius, ColorControlCard, ColorControlBorder, 1)

				aboutText := "ECC 风扇控制器\n\n" +
					"版本：1.0.0 (纯 Go + Gio 自绘桌面引擎)\n" +
					"架构：直接操作笔记本 EC 寄存器，硬件级闭环温度曲线控制\n" +
					"特性：0 浏览器组件、0 WebView2 依赖、超轻工作集内存占用\n\n" +
					"遵循开源协议分发与硬件保护规范。"
				return layout.Inset{Top: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.W.Layout(gtx, k.text(aboutText, textStyle{size: 12, color: ColorTextPrimary}))
				})
			})
		}),
	)
}

// ---------- Toast 覆盖层 ----------

func (w *Window) layoutToast(gtx layout.Context, k *kit) layout.Dimensions {
	w.toastMu.Lock()
	t := w.toast
	w.toastMu.Unlock()
	if !t.visible || t.msg == "" {
		return layout.Dimensions{}
	}

	border, txtCol, prefix := ColorAccentGreen, ColorTextPrimary, "✓ "
	if t.isErr {
		border, txtCol, prefix = ColorDangerRed, ColorToastErrText, "✕ "
	}
	full := prefix + t.msg

	return layout.Inset{Top: unit.Dp(ToastTop)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// 第一次布局仅测量文本自然宽度
			m := op.Record(gtx.Ops)
			dims := k.cardTitle(full, txtCol)(gtx)
			m.Stop()

			tw := dims.Size.X + gtx.Dp(24)*2
			if tw < gtx.Dp(ToastMinW) {
				tw = gtx.Dp(ToastMinW)
			}
			th := dims.Size.Y + gtx.Dp(10)*2
			gtx.Constraints = layout.Exact(image.Pt(tw, th))
			drawRoundRect(gtx, ToastRadius, ColorToastBg, border, 2)
			layout.Center.Layout(gtx, k.cardTitle(full, txtCol))
			return layout.Dimensions{Size: gtx.Constraints.Min}
		})
	})
}
