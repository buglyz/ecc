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
}

func TestValidateStartupTargetRejectsUserWritableLocation(t *testing.T) {
	programFiles := t.TempDir()
	target := filepath.Join(t.TempDir(), "ecc.exe")
	if err := os.WriteFile(target, []byte("test"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("ProgramFiles", programFiles)
	t.Setenv("ProgramW6432", "")

	err := validateStartupTarget(target)
	if !errors.Is(err, ErrUnsafeStartupTarget) {
		t.Fatalf("validateStartupTarget(%q) error=%v, want ErrUnsafeStartupTarget", target, err)
	}
}

func TestPathWithinRejectsPrefixCollision(t *testing.T) {
	root := filepath.Join(`C:\`, "Program Files")
	target := filepath.Join(`C:\`, "Program Files Backup", "ecc.exe")
	if pathWithin(root, target) {
		t.Fatalf("pathWithin(%q, %q)=true, want false", root, target)
	}
}
