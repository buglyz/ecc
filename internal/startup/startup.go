package startup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/buglyz/ecc/internal/process"
)

const Identifier = "风扇控制"

const scheduledTaskTimeout = 5 * time.Second

// ErrUnsafeStartupTarget 表示启动目标无法解析、不存在或不是普通文件。
// 名称保留以兼容既有调用方，调用方据此返回 400。
var ErrUnsafeStartupTarget = errors.New("invalid startup target")

func IsEnabled(identifier string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskTimeout)
	defer cancel()
	cmd := scheduledTaskCommand(ctx, "/Query", "/TN", taskName(identifier))
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run() == nil
}

func Add(targetPath, identifier string) error {
	targetAbs, protected, err := resolveStartupTarget(targetPath)
	if err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}

	// Program Files 下的目标以 HIGHEST 运行（登录免 UAC）；其他目录使用
	// LIMITED，由程序启动时自行请求 UAC，避免提权任务指向用户可写路径。
	runLevel := "LIMITED"
	if protected {
		runLevel = "HIGHEST"
	}

	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskTimeout)
	defer cancel()
	// 登录自启动不打开浏览器，只在托盘常驻；用户可从托盘图标打开控制台。
	cmd := scheduledTaskCommand(ctx, "/Create",
		"/TN", taskName(identifier),
		"/TR", `"`+targetAbs+`" --no-browser`,
		"/SC", "ONLOGON",
		"/RL", runLevel,
		"/RU", u.Username,
		"/F",
	)
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run()
}

func Remove(identifier string) error {
	if !IsEnabled(identifier) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskTimeout)
	defer cancel()
	cmd := scheduledTaskCommand(ctx, "/Delete", "/TN", taskName(identifier), "/F")
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run()
}

func scheduledTaskCommand(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "schtasks.exe", args...)
}

// resolveStartupTarget 解析启动目标，返回规范化绝对路径以及是否位于受保护的
// 安装目录（Program Files）。非受保护路径仍然允许创建开机自启，但使用 LIMITED
// 计划任务，登录时由程序自行请求 UAC。
func resolveStartupTarget(targetPath string) (string, bool, error) {
	resolvedTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return "", false, fmt.Errorf("%w: cannot resolve %q: %v", ErrUnsafeStartupTarget, targetPath, err)
	}
	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return "", false, fmt.Errorf("%w: cannot access %q: %v", ErrUnsafeStartupTarget, resolvedTarget, err)
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("%w: %q is not a regular file", ErrUnsafeStartupTarget, resolvedTarget)
	}
	targetAbs, err := filepath.Abs(resolvedTarget)
	if err != nil {
		return "", false, fmt.Errorf("%w: cannot resolve absolute path for %q: %v", ErrUnsafeStartupTarget, resolvedTarget, err)
	}
	return targetAbs, isProtectedInstallPath(targetAbs), nil
}

func isProtectedInstallPath(targetAbs string) bool {
	for _, root := range protectedInstallRoots() {
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rootAbs, err := filepath.Abs(resolvedRoot)
		if err == nil && pathWithin(rootAbs, targetAbs) {
			return true
		}
	}
	return false
}

func validateStartupTarget(targetPath string) error {
	_, _, err := resolveStartupTarget(targetPath)
	return err
}

// RequiresElevation 报告启动目标是否会以 HIGHEST 权限创建计划任务。
// 非 Program Files 目录返回 false：任务以当前用户权限运行，程序启动时自行请求 UAC。
func RequiresElevation(targetPath string) bool {
	_, protected, err := resolveStartupTarget(targetPath)
	return err == nil && protected
}

func protectedInstallRoots() []string {
	roots := make([]string, 0, 2)
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432")} {
		if root == "" {
			continue
		}
		duplicate := false
		for _, existing := range roots {
			if strings.EqualFold(filepath.Clean(existing), filepath.Clean(root)) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			roots = append(roots, root)
		}
	}
	return roots
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func taskName(identifier string) string {
	return identifier
}
