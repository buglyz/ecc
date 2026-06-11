package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverPopulatesStatePaths(t *testing.T) {
	p := Discover("FanControllerTest")
	if p.AppName != "FanControllerTest" {
		t.Fatalf("AppName=%q, want FanControllerTest", p.AppName)
	}
	if !strings.HasSuffix(p.ConfigPath, filepath.Join("FanControllerTest", "config.json")) {
		t.Fatalf("ConfigPath=%q, want it under the app state dir", p.ConfigPath)
	}
	if !strings.HasSuffix(p.LegacyData, "data.dat") {
		t.Fatalf("LegacyData=%q, want data.dat", p.LegacyData)
	}
	if !strings.HasSuffix(p.LogPath, "fan_controller.log") {
		t.Fatalf("LogPath=%q, want fan_controller.log", p.LogPath)
	}
	if p.StartupTarget == "" {
		t.Fatal("StartupTarget should resolve to the executable path")
	}
	if filepath.Dir(p.ConfigPath) != p.StateDir {
		t.Fatalf("ConfigPath dir=%q, want StateDir=%q", filepath.Dir(p.ConfigPath), p.StateDir)
	}
}

func TestAppLegacyConfigReturnsExecutableDirCandidates(t *testing.T) {
	p := Paths{ExecutableDir: filepath.FromSlash("/opt/app")}
	legacy := AppLegacyConfig(p)
	if len(legacy) != 2 {
		t.Fatalf("len=%d, want 2", len(legacy))
	}
	if filepath.Base(legacy[0]) != "config.json" || filepath.Base(legacy[1]) != "data.dat" {
		t.Fatalf("legacy candidates=%v, want config.json and data.dat", legacy)
	}
}

func TestFirstExistingFallsBackToFirstCandidate(t *testing.T) {
	got := firstExisting("nope-1", "nope-2")
	if got != "nope-1" {
		t.Fatalf("firstExisting=%q, want first candidate when none exist", got)
	}
	if firstExisting() != "" {
		t.Fatal("firstExisting with no candidates should return empty string")
	}
}

func TestFirstExistingPrefersEarliestThatExists(t *testing.T) {
	dir := t.TempDir()
	exeAsset := filepath.Join(dir, "exe", "assets", "ec-probe.exe")
	wdAsset := filepath.Join(dir, "wd", "assets", "ec-probe.exe")
	for _, p := range []string{exeAsset, wdAsset} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Both exist; the exe-dir candidate is listed first and must win, mirroring
	// how Discover orders exeDir/assets ahead of wd/assets so autostart (working
	// dir = System32) still resolves the bundled dependency.
	if got := firstExisting(exeAsset, wdAsset); got != exeAsset {
		t.Fatalf("firstExisting=%q, want exe-dir candidate %q", got, exeAsset)
	}
}
