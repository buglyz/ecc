//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// startWatchdog 以分离进程启动看门狗，主进程被强杀后仍能写回 FF/FF。
func startWatchdog(heartbeatPath string, timeout time.Duration, logger *log.Logger) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{
		"--watchdog",
		"--watchdog-heartbeat", heartbeatPath,
		"--watchdog-timeout", strconv.Itoa(int(timeout / time.Second)),
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	logger.Printf("看门狗已启动: pid=%d", cmd.Process.Pid)
	return cmd.Process.Release()
}
