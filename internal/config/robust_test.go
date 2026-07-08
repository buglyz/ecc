package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buglyz/ecc/internal/paths"
)

// TestParsePickleDictRejectsTruncatedStreamsWithoutPanic ensures a corrupt or
// truncated data.dat cannot crash startup. Each vector previously panicked via
// an unchecked readByte / out-of-range stack slice.
func TestParsePickleDictRejectsTruncatedStreamsWithoutPanic(t *testing.T) {
	vectors := [][]byte{
		{'K'},                         // BININT1 with no following byte
		{0x8c},                        // SHORT_BINUNICODE reading a length byte
		{'q'},                         // BINPUT reading an index byte
		{'h'},                         // BINGET reading an index byte
		{'K', 0x05, '(', 0x85, 'e'},   // mark/stack mismatch via APPENDS
		{0x80, 0x04, 0x95},            // PROTO + truncated FRAME
		{},                            // empty
		{0xff, 0xff, 0xff},            // unsupported opcodes
		{'X', 0xff, 0xff, 0xff, 0x7f}, // BINUNICODE with absurd length
	}
	for i, data := range vectors {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("vector %d panicked: %v", i, r)
				}
			}()
			if _, err := parsePickleDict(data); err == nil {
				t.Fatalf("vector %d: expected error for corrupt pickle, got nil", i)
			}
		}()
	}
}

// TestLoadCorruptJSONFallsBackToPickle verifies that a corrupt primary
// config.json does not discard a migratable legacy data.dat: Load must keep
// trying the pickle candidates instead of returning Default() immediately.
func TestLoadCorruptJSONFallsBackToPickle(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	datPath := filepath.Join(dir, "data.dat")

	if err := os.WriteFile(cfgPath, []byte("{this is not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// realPickle (defined in pickle_test.go) is a genuine pickle dict whose
	// manual_speed is 77; the default is 50, so the value distinguishes the
	// migration path from a bail-to-default.
	if err := os.WriteFile(datPath, realPickle, 0o644); err != nil {
		t.Fatal(err)
	}

	result := Load(paths.Paths{StateDir: dir, ConfigPath: cfgPath, LegacyData: datPath})
	cfg := result.Config
	if cfg.ManualSpeed != 77 {
		t.Fatalf("manual_speed=%d, want 77 from migrated pickle (corrupt JSON must not shadow legacy data)", cfg.ManualSpeed)
	}
}
