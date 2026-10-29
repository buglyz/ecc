//go:build !windows

package nl5

const (
	KeyPath               = `SOFTWARE\WOW6432Node\NL5\ControlCenter`
	ValueFanControlStatus = "FanControlStatus"
)

func FanControlStatus() (uint32, bool) { return 0, false }

func SetFanControlStatus(value uint32) bool { return false }
