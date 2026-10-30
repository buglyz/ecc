package controller

import "time"

const (
	AppName   = "FanController"
	ECRegFan1 = "0x2C"
	ECRegFan2 = "0x2D"
	// Fan1/Fan2 实际转速寄存器（小端 16 位，低字节在前）。经真机 dump 交叉验证：
	// 强制 Fan1 0%→100% 时 0xB0/0xB1 从 0 跳到 ~3800，空闲态 ~5000 rpm，且与
	// 机器自带控制台读数一致。此前误用的 0xD0-0xD3 恒为常量 99、强制满速纹丝不动，
	// 根本不是 RPM 寄存器——那条路径因当时接的 GMWMI reader 忽略入参而从未真正跑过。
	ECRegFan1RPMLow  = "0xB0"
	ECRegFan1RPMHigh = "0xB1"
	ECRegFan2RPMLow  = "0xB2"
	ECRegFan2RPMHigh = "0xB3"
	ECFanRelease     = "0xFF"
	SamplesPerCycle  = 6
	SampleInterval   = time.Second
	HysteresisTemp   = 1.5
	// 高温硬保护：达到阈值后写 0xFF 交还 EC 曲线，并在回差内保持。
	// 参考 QNLXS/S8D6 的实测阈值（CPU 92°C / GPU 87°C）。
	PanicCPUTemp = 92.0
	PanicGPUTemp = 87.0
	// 高温保护解除所需的回差：CPU < 92-5、GPU < 87-5 才恢复普通控制。
	PanicReleaseMargin = 5.0
	// 0xFF 表示“不限制”，交还 EC 原厂曲线；比写 100 更可靠。
	SafetyReleaseValue  = 0xFF
	LoopDriftTolerance  = 5 * time.Second
	ExpectedCycleJitter = 1300 * time.Millisecond
	// 心跳写入：温度稳定时也至少每 10 秒同步一次，防止固件收回控制权。
	HeartbeatInterval = 10 * time.Second
	// RPMReadInterval 限制 RPM 硬件查询频率。RPM 读取要各起一次 ec-probe.exe 进程
	// （两个字节两次进程，每次都加载/卸载一次 WinRing0 驱动，开销可达数十~数百毫秒），
	// 无需每次采样都做，节流到该间隔，两次之间复用上次结果，避免每秒一轮进程启动的持续开销。
	RPMReadInterval   = 5 * time.Second
	HistoryMaxSamples = 28800
	// NoTempFailSafeCycles：连续多少个周期拿不到任何有效温度后，判定温度数据
	// 已不可信，主动释放 EC 控制权交还固件，避免风扇长期停在最后写入的低速档。
	// 按默认 6 样本/周期、1 秒/样本计算约 18 秒。
	NoTempFailSafeCycles = 3
	CPUWeight            = 0.7
	CurvePoints          = 5
	CurveTempMin         = 30.0
	CurveTempMax         = 100.0
	CurveSpeedMin        = 0.0
	CurveSpeedMax        = 100.0
	DefaultStrategy      = "weighted"
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

func ExpectedCycleDuration(pollInterval time.Duration) time.Duration {
	return time.Duration(SamplesPerCycle)*pollInterval + ExpectedCycleJitter
}
