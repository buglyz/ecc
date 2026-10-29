//go:build !windows

package pawnio

import (
	"context"
	"errors"
	"log"
)

// Device 是非 Windows 平台的占位实现；ECC 实际只发布 Windows 版本。
type Device struct{}

func Open(modulePath string, logger *log.Logger) (*Device, error) {
	return nil, errors.New("PawnIO is only supported on Windows")
}

func (d *Device) ReadEC(ctx context.Context, reg byte) (byte, error) {
	return 0, errors.New("PawnIO is only supported on Windows")
}

func (d *Device) WriteEC(ctx context.Context, reg, val byte) error {
	return errors.New("PawnIO is only supported on Windows")
}

func (d *Device) Close() error { return nil }
