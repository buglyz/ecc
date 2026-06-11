package paths

import (
	"os"
	"path/filepath"
)

type Paths struct {
	AppName       string `json:"app_name"`
	ExecutableDir string `json:"executable_dir"`
	WorkingDir    string `json:"working_dir"`
	StateDir      string `json:"state_dir"`
	ECProbe       string `json:"ec_probe"`
	HardwareDLL   string `json:"hardware_dll"`
	ConfigPath    string `json:"config_path"`
	LegacyData    string `json:"legacy_data"`
	LogPath       string `json:"log_path"`
	StartupTarget string `json:"startup_target"`
}

func Discover(appName string) Paths {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exe, _ = filepath.Abs(exe)
	exeDir := filepath.Dir(exe)
	wd, err := os.Getwd()
	if err != nil {
		wd = exeDir
	}
	wd, _ = filepath.Abs(wd)
	stateRoot := os.Getenv("LOCALAPPDATA")
	if stateRoot == "" {
		stateRoot = exeDir
	}
	stateDir := filepath.Join(stateRoot, appName)
	return Paths{
		AppName:       appName,
		ExecutableDir: exeDir,
		WorkingDir:    wd,
		StateDir:      stateDir,
		// 候选路径优先基于 exe 目录（exeDir），而非工作目录（wd）：
		// 开机自启（任务计划程序）启动时工作目录是 System32，只有相对 exe 目录
		// 才能稳定找到随 exe 分发的 assets/ 依赖。wd 候选保留作开发场景后备。
		ECProbe: firstExisting(
			filepath.Join(exeDir, "ec-probe.exe"),
			filepath.Join(exeDir, "assets", "ec-probe.exe"),
			filepath.Join(exeDir, "_internal", "ec-probe.exe"),
			filepath.Join(filepath.Dir(exeDir), "ec-probe.exe"),
			filepath.Join(wd, "assets", "ec-probe.exe"),
			filepath.Join(wd, "ec-probe.exe"),
		),
		HardwareDLL: firstExisting(
			filepath.Join(exeDir, "_internal", "data", "LibreHardwareMonitorLib.dll"),
			filepath.Join(exeDir, "data", "LibreHardwareMonitorLib.dll"),
			filepath.Join(exeDir, "assets", "LibreHardwareMonitorLib.dll"),
			filepath.Join(exeDir, "LibreHardwareMonitorLib.dll"),
			filepath.Join(wd, "assets", "LibreHardwareMonitorLib.dll"),
			filepath.Join(wd, "LibreHardwareMonitorLib.dll"),
		),
		ConfigPath:    filepath.Join(stateDir, "config.json"),
		LegacyData:    filepath.Join(stateDir, "data.dat"),
		LogPath:       filepath.Join(stateDir, "fan_controller.log"),
		StartupTarget: exe,
	}
}

func AppLegacyConfig(paths Paths) []string {
	return []string{
		filepath.Join(paths.ExecutableDir, "config.json"),
		filepath.Join(paths.ExecutableDir, "data.dat"),
	}
}

func firstExisting(candidates ...string) string {
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}
