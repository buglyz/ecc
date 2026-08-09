package controller

import (
	"testing"
	"time"
)

// BenchmarkInterpolateCurve 测量控制循环写决策路径的单次插值代价。
// curve-sorted 用归一化后的曲线（config.Normalize 恒定保证有序），走零分配快路径；
// curve-malformed 故意传入乱序曲线，触发防御性拷贝+排序，用于对比两条路径的分配差异。
func BenchmarkInterpolateCurve(b *testing.B) {
	sorted := append([]Point(nil), DefaultCurve...)
	malformed := []Point{{70, 60}, {40, 30}, {90, 100}, {80, 85}, {55, 40}}

	b.Run("curve-sorted", func(b *testing.B) {
		b.ReportAllocs()
		var sink float64
		for i := 0; i < b.N; i++ {
			// 命中中部段（55-70°C），走完整线性插值分支。
			sink = InterpolateCurve(sorted, 62.5)
		}
		_ = sink
	})

	b.Run("curve-malformed", func(b *testing.B) {
		b.ReportAllocs()
		var sink float64
		for i := 0; i < b.N; i++ {
			sink = InterpolateCurve(malformed, 62.5)
		}
		_ = sink
	})
}

// BenchmarkSnapshotSince 测量仪表盘拉取时间窗口（如 30 分钟）历史的代价。
// hot cutoff 命中环形缓冲尾部，只分配窗口大小的切片。
func BenchmarkSnapshotSince(b *testing.B) {
	const warmup = 6000 // 越过两次 28800 上限太慢，这里用「近期写入、尾部命中」的形态即可
	now := time.Now()
	h := NewHistory(HistoryMaxSamples)
	for i := 0; i < warmup; i++ {
		h.Add(HistorySample{Time: now.Add(-time.Duration(warmup-i) * time.Second), Speed: i})
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = h.SnapshotSince(now.Add(-30 * time.Minute))
	}
}

// BenchmarkHistoryAdd 测量每采样一次的历史追加代价（纯环形缓冲写入，无分配上限内）。
func BenchmarkHistoryAdd(b *testing.B) {
	h := NewHistory(HistoryMaxSamples)
	sample := HistorySample{Time: time.Now(), Speed: 50}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Add(sample)
	}
}