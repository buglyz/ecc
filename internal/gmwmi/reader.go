package gmwmi

import (
	"context"
	"log"
	"sync"
)

// Reader 通过 GMWMI 读取风扇 RPM。ECC 控制器只需要 CPU RPM 作为展示，
// 因此复用 controller.FanReader 的既有接口。
type Reader struct {
	client *Client
	caps   Capabilities
	logger *log.Logger

	mu   sync.Mutex
	last Sensors
}

func NewReader(client *Client, caps Capabilities, logger *log.Logger) *Reader {
	if logger == nil {
		logger = log.Default()
	}
	return &Reader{client: client, caps: caps, logger: logger}
}

func (r *Reader) ReadRPM(ctx context.Context, registerLow, registerHigh string) (uint16, bool) {
	s, err := r.client.ReadSensors(ctx, r.caps)
	if err != nil {
		r.logger.Printf("GMWMI 读取转速失败: %v", err)
		return 0, false
	}
	r.mu.Lock()
	r.last = s
	r.mu.Unlock()

	if s.CPURPM == nil || *s.CPURPM == 0 {
		return 0, false
	}
	return *s.CPURPM, true
}

func (r *Reader) LastSensors() Sensors {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}
