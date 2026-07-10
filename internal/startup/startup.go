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

var ErrUnsafeStartupTarget = errors.New("startup target must be installed under Program Files")

func IsEnabled(identifier string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskTimeout)
	defer cancel()
	cmd := scheduledTaskCommand(ctx, "/Query", "/TN", taskName(identifier))
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	return cmd.Run() == nil
}

func Add(targetPath, identifier string) error {
	if err := validateStartupTarget(targetPath); err != nil {
		return err
	}
	u, err := user.Current()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), scheduledTaskTimeout)
	defer cancel()
	cmd := scheduledTaskCommand(ctx, "/Create",
		"/TN", taskName(identifier),
		"/TR", `"`+targetPath+`"`,
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
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

func validateStartupTarget(targetPath string) error {
	resolvedTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return fmt.Errorf("%w: cannot resolve %q: %v", ErrUnsafeStartupTarget, targetPath, err)
	}
	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return fmt.Errorf("%w: cannot access %q: %v", ErrUnsafeStartupTarget, resolvedTarget, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q is not a regular file", ErrUnsafeStartupTarget, resolvedTarget)
	}
	targetAbs, err := filepath.Abs(resolvedTarget)
	if err != nil {
		return fmt.Errorf("%w: cannot resolve absolute path for %q: %v", ErrUnsafeStartupTarget, resolvedTarget, err)
	}
	for _, root := range protectedInstallRoots() {
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rootAbs, err := filepath.Abs(resolvedRoot)
		if err == nil && pathWithin(rootAbs, targetAbs) {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is outside Program Files", ErrUnsafeStartupTarget, targetAbs)
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
