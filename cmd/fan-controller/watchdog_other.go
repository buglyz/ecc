//go:build !windows

package main

import (
	"log"
	"time"
)

func startWatchdog(heartbeatPath string, timeout time.Duration, logger *log.Logger) error {
	return nil
}
