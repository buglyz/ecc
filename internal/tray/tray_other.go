//go:build !windows

package tray

type Tray struct{}

func New(onShow, onExit func()) *Tray            { return &Tray{} }
func (t *Tray) Run()                             {}
func (t *Tray) Update(temp *float64, speed *int) {}
func (t *Tray) Alert()                           {}
func (t *Tray) Stop()                            {}
