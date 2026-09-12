//go:build !windows

package instance

func Acquire() (release func(), alreadyRunning bool, err error) {
	return func() {}, false, nil
}
