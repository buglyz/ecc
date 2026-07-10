package controller

import (
	"sync"
	"time"
)

type History struct {
	mu    sync.Mutex
	max   int
	buf   []HistorySample
	head  int
	count int
}

func NewHistory(max int) *History {
	return &History{max: max, buf: make([]HistorySample, max)}
}

func (h *History) Add(sample HistorySample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf[h.head] = sample
	h.head = (h.head + 1) % h.max
	if h.count < h.max {
		h.count++
	}
}

func (h *History) Snapshot() []HistorySample {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]HistorySample, h.count)
	if h.count < h.max {
		copy(out, h.buf[:h.count])
	} else {
		n := copy(out, h.buf[h.head:])
		copy(out[n:], h.buf[:h.head])
	}
	return out
}

// SnapshotSince 返回 Time >= cutoff 的样本，按时间升序排列。
// 样本按时间递增写入，因此从最新样本向后回溯，命中早于 cutoff 即停止，
// 只分配窗口大小而非整个环形缓冲。cutoff 为零值时返回全部样本。
func (h *History) SnapshotSince(cutoff time.Time) []HistorySample {
	h.mu.Lock()
	defer h.mu.Unlock()

	count := h.count
	if !cutoff.IsZero() {
		count = 0
		for i := 0; i < h.count; i++ {
			// idx 从最新样本向后回溯：head 指向下一个写入位，head-1 为最新。
			idx := (h.head - 1 - i + h.max) % h.max
			if h.buf[idx].Time.Before(cutoff) {
				break
			}
			count++
		}
	}

	out := make([]HistorySample, count)
	start := (h.head - count + h.max) % h.max
	for i := range out {
		out[i] = h.buf[(start+i)%h.max]
	}
	return out
}
