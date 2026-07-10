package sensors

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestNewPowerShellCommandUsesEncodedScriptAndControlledDLLPath(t *testing.T) {
	dllPath := `C:\Program Files\ECC\assets\LibreHardwareMonitorLib.dll`
	cmd := newPowerShellCommand(dllPath)

	if hasArgument(cmd.Args, "-File") {
		t.Fatalf("PowerShell command must not execute a script file: %v", cmd.Args)
	}
	encodedIndex := argumentIndex(cmd.Args, "-EncodedCommand")
	if encodedIndex == -1 || encodedIndex+1 >= len(cmd.Args) {
		t.Fatalf("PowerShell command missing -EncodedCommand: %v", cmd.Args)
	}
	if got := decodeEncodedPowerShell(t, cmd.Args[encodedIndex+1]); !strings.Contains(got, "$env:ECC_LHM_DLL_PATH") {
		t.Fatalf("encoded script does not read the controlled DLL path: %q", got)
	}

	var values []string
	for _, entry := range cmd.Env {
		if strings.EqualFold(strings.SplitN(entry, "=", 2)[0], lhmDLLPathEnv) {
			values = append(values, entry)
		}
	}
	if len(values) != 1 || values[0] != lhmDLLPathEnv+"="+dllPath {
		t.Fatalf("DLL path environment=%v, want exactly %q", values, lhmDLLPathEnv+"="+dllPath)
	}
}

func TestWithoutEnvironmentVariableRemovesCaseInsensitiveDuplicates(t *testing.T) {
	env := []string{"PATH=x", "ecc_lhm_dll_path=old", "ECC_LHM_DLL_PATH=older"}
	got := withoutEnvironmentVariable(env, lhmDLLPathEnv)
	if len(got) != 1 || got[0] != "PATH=x" {
		t.Fatalf("filtered environment=%v, want [PATH=x]", got)
	}
}

func hasArgument(args []string, value string) bool {
	return argumentIndex(args, value) != -1
}

func argumentIndex(args []string, value string) int {
	for i, arg := range args {
		if arg == value {
			return i
		}
	}
	return -1
}

func decodeEncodedPowerShell(t *testing.T, encoded string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	if len(data)%2 != 0 {
		t.Fatalf("encoded PowerShell byte length=%d, want even", len(data))
	}
	values := make([]uint16, len(data)/2)
	for i := range values {
		values[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return string(utf16.Decode(values))
}
