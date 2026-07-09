package controller

import "time"

const (
	AppName             = "FanController"
	ECRegFan1           = "0x2C"
	ECRegFan2           = "0x2D"
	ECRegFan1RPMLow     = "0xD0"
	ECRegFan1RPMHigh    = "0xD1"
	ECRegFan2RPMLow     = "0xD2"
	ECRegFan2RPMHigh    = "0xD3"
	ECFanRelease        = "0xFF"
	SamplesPerCycle     = 6
	SampleInterval      = time.Second
	HysteresisTemp      = 2.0
	LoopDriftTolerance  = 5 * time.Second
	ExpectedCycleJitter = 1300 * time.Millisecond
	HeartbeatInterval   = 30 * time.Second
	// RPMReadInterval 限制 RPM 硬件查询频率。RPM 读取（GMWMI 走一次完整 WMI
	// 查询，开销可达数十~数百毫秒）无需每次采样都做，节流到该间隔，两次之间
	// 复用上次结果，避免每秒一次 WMI 查询带来的持续 CPU 开销。
	RPMReadInterval   = 5 * time.Second
	HistoryMaxSamples = 28800
	CPUWeight         = 0.7
	CurvePoints       = 5
	CurveTempMin      = 30.0
	CurveTempMax      = 100.0
	CurveSpeedMin     = 0.0
	CurveSpeedMax     = 100.0
	DefaultStrategy   = "weighted"
)

var DefaultCurve = []Point{{40, 30}, {55, 40}, {70, 60}, {80, 85}, {90, 100}}

var Strategies = []Strategy{
	{Key: "weighted", Label: "加权 (0.7·CPU + 0.3·GPU)"},
	{Key: "max", Label: "取最大值 max(CPU, GPU)"},
	{Key: "cpu", Label: "仅 CPU"},
	{Key: "gpu", Label: "仅 GPU"},
}

var Presets = []Preset{
	{Key: "silent", Label: "静音", Curve: []Point{{40, 15}, {55, 25}, {70, 40}, {80, 60}, {90, 85}}, Strategy: "weighted"},
	{Key: "balanced", Label: "平衡", Curve: []Point{{40, 30}, {55, 40}, {70, 60}, {80, 85}, {90, 100}}, Strategy: "weighted"},
	{Key: "performance", Label: "性能", Curve: []Point{{40, 40}, {55, 60}, {65, 80}, {75, 95}, {85, 100}}, Strategy: "max"},
}

func ExpectedCycleDuration() time.Duration {
	return time.Duration(SamplesPerCycle)*SampleInterval + ExpectedCycleJitter
}
