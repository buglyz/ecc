package startup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateStartupTargetAllowsProgramFilesExecutable(t *testing.T) {
	programFiles := t.TempDir()
	target := filepath.Join(programFiles, "ECC", "ecc.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(target, []byte("test"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("ProgramFiles", programFiles)
	t.Setenv("ProgramW6432", "")

	if err := validateStartupTarget(target); err != nil {
		t.Fatalf("validateStartupTarget(%q): %v", target, err)
	}
	if !RequiresElevation(target) {
		t.Fatal("RequiresElevation = false for Program Files target, want true")
	}
}

// Program Files 之外的目录也应允许创建开机自启，但使用 LIMITED 计划任务。
func TestResolveStartupTargetAllowsOutsideProgramFiles(t *testing.T) {
	programFiles := t.TempDir()
	target := filepath.Join(t.TempDir(), "ecc.exe")
	if err := os.WriteFile(target, []byte("test"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("ProgramFiles", programFiles)
	t.Setenv("ProgramW6432", "")

	abs, protected, err := resolveStartupTarget(target)
	if err != nil {
		t.Fatalf("resolveStartupTarget(%q): %v", target, err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("resolved path %q is not absolute", abs)
	}
	if protected {
		t.Fatal("protected = true outside Program Files, want false")
	}
	if RequiresElevation(target) {
		t.Fatal("RequiresElevation = true outside Program Files, want false")
	}
}

func TestResolveStartupTargetRejectsMissingFile(t *testing.T) {
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("ProgramW6432", "")
	missing := filepath.Join(t.TempDir(), "missing.exe")
	if _, _, err := resolveStartupTarget(missing); !errors.Is(err, ErrUnsafeStartupTarget) {
		t.Fatalf("resolveStartupTarget(%q) error=%v, want ErrUnsafeStartupTarget", missing, err)
	}
}

func TestResolveStartupTargetRejectsDirectory(t *testing.T) {
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("ProgramW6432", "")
	dir := t.TempDir()
	if _, _, err := resolveStartupTarget(dir); !errors.Is(err, ErrUnsafeStartupTarget) {
		t.Fatalf("resolveStartupTarget(%q) error=%v, want ErrUnsafeStartupTarget", dir, err)
	}
}

func TestPathWithinRejectsPrefixCollision(t *testing.T) {
	root := filepath.Join(`C:\`, "Program Files")
	target := filepath.Join(`C:\`, "Program Files Backup", "ecc.exe")
	if pathWithin(root, target) {
		t.Fatalf("pathWithin(%q, %q)=true, want false", root, target)
	}
}
