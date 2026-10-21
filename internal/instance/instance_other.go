//go:build !windows

package instance

// 非 Windows 平台没有本项目实际运行场景，保留空实现以便跨平台编译与测试。

const DefaultMutexName = "ecc-fan-controller-single-instance"

type Guard struct{}

func Acquire(string) (*Guard, bool, error) {
	return &Guard{}, true, nil
}

func (g *Guard) Release() {}
