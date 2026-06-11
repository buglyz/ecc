package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

const maxLogBytes = 512 * 1024
const logBackups = 3

func New(logPath string) (*log.Logger, func()) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		logger := log.New(os.Stderr, "", log.LstdFlags)
		logger.Printf("日志目录创建失败: %v", err)
		return logger, func() {}
	}
	rotate(logPath)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		logger := log.New(os.Stderr, "", log.LstdFlags)
		logger.Printf("日志打开失败: %v", err)
		return logger, func() {}
	}
	logger := log.New(io.MultiWriter(file, os.Stderr), "", log.LstdFlags)
	logger.Printf("日志初始化完成: %s", logPath)
	return logger, func() { _ = file.Close() }
}

func rotate(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < maxLogBytes {
		return
	}
	for i := logBackups - 1; i >= 1; i-- {
		oldName := logPath + "." + strconv.Itoa(i)
		newName := logPath + "." + strconv.Itoa(i+1)
		_ = os.Rename(oldName, newName)
	}
	_ = os.Rename(logPath, logPath+".1")
}
