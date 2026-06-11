package sensors

import (
	"math"
	"sync"
	"time"

	"github.com/buglyz/ecc/internal/controller"
)

type SimulatedReader struct {
	mu    sync.Mutex
	start time.Time
}

func NewSimulatedReader() *SimulatedReader {
	return &SimulatedReader{start: time.Now()}
}

func (r *SimulatedReader) Read() controller.Temps {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.start.IsZero() {
		r.start = time.Now()
	}
	seconds := time.Since(r.start).Seconds()
	cpu := 62.0 + math.Sin(seconds/18)*12
	gpu := 55.0 + math.Cos(seconds/23)*10
	return controller.Temps{CPU: &cpu, GPU: &gpu}
}

func (r *SimulatedReader) Close() error {
	return nil
}
