package ui

import (
	"fmt"

	"image/color"

	"github.com/buglyz/ecc/internal/controller"
)

// 本文件存放与渲染无关的纯逻辑: 曲线校验、数值格式化。
// 不带构建标签, 保证测试与跨平台编译可用。

// ValidateCurve 校验曲线: 点数必须为 controller.CurvePoints, 每个点在
// 温度/转速范围内, 且温度严格递增(相邻点至少相差 1°C)。
func ValidateCurve(curve []controller.Point) error {
	if len(curve) != controller.CurvePoints {
		return fmt.Errorf("曲线必须包含 %d 个控制点, 当前为 %d 个", controller.CurvePoints, len(curve))
	}
	for i, p := range curve {
		if p.Temp() < controller.CurveTempMin || p.Temp() > controller.CurveTempMax {
			return fmt.Errorf("控制点 #%d 温度 %.1f°C 超出范围 [%.0f, %.0f]", i+1, p.Temp(), controller.CurveTempMin, controller.CurveTempMax)
		}
		if p.Speed() < controller.CurveSpeedMin || p.Speed() > controller.CurveSpeedMax {
			return fmt.Errorf("控制点 #%d 转速 %.0f%% 超出范围 [%.0f, %.0f]", i+1, p.Speed(), controller.CurveSpeedMin, controller.CurveSpeedMax)
		}
		if i > 0 && p.Temp() <= curve[i-1].Temp()+1.0 {
			return fmt.Errorf("控制点 #%d 温度 %.1f°C 必须大于前一点 %.1f°C (至少 1°C)", i+1, p.Temp(), curve[i-1].Temp())
		}
	}
	return nil
}

func formatTemp(t *float64) string {
	if t == nil {
		return "--"
	}
	return fmt.Sprintf("%.1f°C", *t)
}

func formatSpeed(s *int) string {
	if s == nil {
		return "--"
	}
	return fmt.Sprintf("%d%%", *s)
}

func formatRPM(rpm *uint16) string {
	if rpm == nil || *rpm == 0 {
		return "RPM: 不可用"
	}
	return fmt.Sprintf("实际: %d RPM", *rpm)
}

// tempColor 按温度分级选色: <60 绿, <80 黄, ≥80 红, 未知用主文字色。
func tempColor(t *float64) color.NRGBA {
	if t == nil {
		return ColorTextPrimary
	}
	if *t < 60 {
		return ColorAccentGreen
	}
	if *t < 80 {
		return ColorWarningYellow
	}
	return ColorDangerRed
}
