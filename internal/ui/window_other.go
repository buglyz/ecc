//go:build !windows

package ui

import (
	"fmt"

	"github.com/buglyz/ecc/internal/app"
)

type Window struct{}

func New(_ *app.Service, _ func(), _ bool) *Window { return &Window{} }

func (w *Window) Start(_ bool) error { return fmt.Errorf("原生 UI 仅支持 Windows") }

func (w *Window) Show() {}

func (w *Window) Close() {}

func ActivateExisting() bool { return false }
