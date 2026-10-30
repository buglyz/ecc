package controller

import "math"

const (
	ControlModeCurve  = "curve"
	ControlModeTarget = "target"

	DefaultTargetTemp       = 66.0
	DefaultTargetHysteresis = 4.0
	DefaultTargetKP         = 5.0
	DefaultTargetBias       = 55
	DefaultTargetMin        = 45
	DefaultTargetMax        = 100
	DefaultTargetUpStep     = 30
	DefaultTargetDownStep   = 10
)

// TargetConfig 是“目标温度模式”的参数。
// 控制律：cap = Bias + KP × (temp - Target)，再钳到 [Min, Max]，
// 在 Target ± Hysteresis 内保持不动，并限制每周期上调/下调幅度。
type TargetConfig struct {
	Temp       float64
	Hysteresis float64
	KP         float64
	Bias       int
	Min        int
	Max        int
	UpStep     int
	DownStep   int
}

func DefaultTargetConfig() TargetConfig {
	return TargetConfig{
		Temp:       DefaultTargetTemp,
		Hysteresis: DefaultTargetHysteresis,
		KP:         DefaultTargetKP,
		Bias:       DefaultTargetBias,
		Min:        DefaultTargetMin,
		Max:        DefaultTargetMax,
		UpStep:     DefaultTargetUpStep,
		DownStep:   DefaultTargetDownStep,
	}
}

func ValidControlMode(mode string) bool {
	return mode == ControlModeCurve || mode == ControlModeTarget
}

func NormalizeTargetConfig(t TargetConfig) TargetConfig {
	if t == (TargetConfig{}) {
		return DefaultTargetConfig()
	}
	if t.Temp == 0 {
		t.Temp = DefaultTargetTemp
	}
	t.Temp = clampFloat(t.Temp, 40, 95)
	if t.Hysteresis == 0 {
		t.Hysteresis = DefaultTargetHysteresis
	}
	t.Hysteresis = clampFloat(t.Hysteresis, 1, 10)
	if t.KP == 0 {
		t.KP = DefaultTargetKP
	}
	t.KP = clampFloat(t.KP, 1, 20)
	t.Bias = clampIntValue(t.Bias, 0, 100)
	t.Min = clampIntValue(t.Min, 0, 100)
	if t.Max <= 0 {
		t.Max = DefaultTargetMax
	}
	t.Max = clampIntValue(t.Max, 0, 100)
	if t.Max < t.Min {
		t.Max = t.Min
	}
	if t.UpStep <= 0 {
		t.UpStep = DefaultTargetUpStep
	}
	if t.DownStep <= 0 {
		t.DownStep = DefaultTargetDownStep
	}
	t.UpStep = clampIntValue(t.UpStep, 1, 100)
	t.DownStep = clampIntValue(t.DownStep, 1, 100)
	return t
}

// targetControlStep 计算下一周期的风扇上限。current 是当前已写入的上限。
// 首次接管（applied=false）时从 Bias 起步，避免从 0 或旧曲线值突然跳变。
func targetControlStep(current int, temp float64, cfg TargetConfig, applied bool) int {
	if !applied {
		current = cfg.Bias
	}
	if math.Abs(temp-cfg.Temp) <= cfg.Hysteresis {
		return clampIntValue(current, 0, 100)
	}

	desired := int(math.Round(float64(cfg.Bias) + cfg.KP*(temp-cfg.Temp)))
	desired = clampIntValue(desired, cfg.Min, cfg.Max)

	if desired > current {
		if desired-current > cfg.UpStep {
			desired = current + cfg.UpStep
		}
	} else if desired < current {
		if current-desired > cfg.DownStep {
			desired = current - cfg.DownStep
		}
	}
	return clampIntValue(desired, 0, 100)
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampIntValue(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
