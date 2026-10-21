//go:build !windows

package instance

// 非 Windows 平台保留空实现以便跨平台编译与测试。

const DefaultShutdownEventName = "ecc-fan-controller-shutdown"

type ShutdownListener struct {
	done chan struct{}
}

func NewShutdownListener(string) (*ShutdownListener, error) {
	return &ShutdownListener{done: make(chan struct{})}, nil
}

func (l *ShutdownListener) Done() <-chan struct{} { return l.done }

func (l *ShutdownListener) Close() {}

func SignalShutdown(string) error { return nil }
