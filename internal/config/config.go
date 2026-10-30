package config

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

type PresetConfig struct {
	Label    string             `json:"label"`
	Curve    []controller.Point `json:"curve"`
	Strategy string             `json:"strategy"`
}

type Config struct {
	Curve            []controller.Point      `json:"curve"`
	Strategy         string                  `json:"strategy"`
	ControlMode      string                  `json:"control_mode"`
	TargetTemp       float64                 `json:"target_temp"`
	TargetHysteresis float64                 `json:"target_hysteresis"`
	TargetKP         float64                 `json:"target_kp"`
	TargetBias       int                     `json:"target_bias"`
	TargetMin        int                     `json:"target_min"`
	TargetMax        int                     `json:"target_max"`
	TargetUpStep     int                     `json:"target_up_step"`
	TargetDownStep   int                     `json:"target_down_step"`
	Theme            string                  `json:"theme"`
	TimeEntry        string                  `json:"time_entry"`
	Minimize         int                     `json:"minimize"`
	ManualEnabled    bool                    `json:"manual_enabled"`
	ManualSpeed      int                     `json:"manual_speed"`
	ActivePreset     string                  `json:"active_preset"`
	Presets          map[string]PresetConfig `json:"presets"`
}

func Default() Config {
	return Config{
		Curve:            cloneCurve(controller.DefaultCurve),
		Strategy:         controller.DefaultStrategy,
		ControlMode:      controller.ControlModeCurve,
		TargetTemp:       controller.DefaultTargetTemp,
		TargetHysteresis: controller.DefaultTargetHysteresis,
		TargetKP:         controller.DefaultTargetKP,
		TargetBias:       controller.DefaultTargetBias,
		TargetMin:        controller.DefaultTargetMin,
		TargetMax:        controller.DefaultTargetMax,
		TargetUpStep:     controller.DefaultTargetUpStep,
		TargetDownStep:   controller.DefaultTargetDownStep,
		Theme:            "light",
		TimeEntry:        "5",
		Minimize:         0,
		ManualEnabled:    false,
		ManualSpeed:      50,
		ActivePreset:     "balanced",
		Presets:          defaultPresets(),
	}
}

func Save(p paths.Paths, cfg Config) error {
	if err := os.MkdirAll(p.StateDir, 0o755); err != nil {
		return err
	}
	tmp := p.ConfigPath + ".tmp"
	data, err := json.MarshalIndent(Normalize(cfg), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.ConfigPath); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func Normalize(cfg Config) Config {
	def := Default()
	if cfg.Curve == nil {
		cfg.Curve = def.Curve
	}
	if cfg.Presets == nil {
		cfg.Presets = def.Presets
	}
	cfg.Curve = normalizeCurve(cfg.Curve)
	cfg.Presets = normalizePresets(cfg.Presets)
	if _, ok := cfg.Presets[cfg.ActivePreset]; !ok {
		cfg.ActivePreset = "balanced"
	}
	if !controller.ValidStrategy(cfg.Strategy) {
		cfg.Strategy = controller.DefaultStrategy
	}
	if !controller.ValidControlMode(cfg.ControlMode) {
		cfg.ControlMode = controller.ControlModeCurve
	}
	tc := controller.NormalizeTargetConfig(controller.TargetConfig{
		Temp:       cfg.TargetTemp,
		Hysteresis: cfg.TargetHysteresis,
		KP:         cfg.TargetKP,
		Bias:       cfg.TargetBias,
		Min:        cfg.TargetMin,
		Max:        cfg.TargetMax,
		UpStep:     cfg.TargetUpStep,
		DownStep:   cfg.TargetDownStep,
	})
	cfg.TargetTemp = tc.Temp
	cfg.TargetHysteresis = tc.Hysteresis
	cfg.TargetKP = tc.KP
	cfg.TargetBias = tc.Bias
	cfg.TargetMin = tc.Min
	cfg.TargetMax = tc.Max
	cfg.TargetUpStep = tc.UpStep
	cfg.TargetDownStep = tc.DownStep
	if cfg.Theme != "dark" {
		cfg.Theme = "light"
	}
	cfg.ManualSpeed = int(clamp(float64(cfg.ManualSpeed), 0, 100))
	if cfg.TimeEntry == "" {
		cfg.TimeEntry = "5"
	}
	minutes, err := strconv.Atoi(cfg.TimeEntry)
	if err != nil {
		cfg.TimeEntry = "5"
	} else {
		cfg.TimeEntry = strconv.Itoa(int(clamp(float64(minutes), 1, 480)))
	}
	if cfg.Minimize != 1 {
		cfg.Minimize = 0
	}
	return cfg
}

func (c Config) Clone() Config {
	return Config{
		Curve:            cloneCurve(c.Curve),
		Strategy:         c.Strategy,
		ControlMode:      c.ControlMode,
		TargetTemp:       c.TargetTemp,
		TargetHysteresis: c.TargetHysteresis,
		TargetKP:         c.TargetKP,
		TargetBias:       c.TargetBias,
		TargetMin:        c.TargetMin,
		TargetMax:        c.TargetMax,
		TargetUpStep:     c.TargetUpStep,
		TargetDownStep:   c.TargetDownStep,
		Theme:            c.Theme,
		TimeEntry:        c.TimeEntry,
		Minimize:         c.Minimize,
		ManualEnabled:    c.ManualEnabled,
		ManualSpeed:      c.ManualSpeed,
		ActivePreset:     c.ActivePreset,
		Presets:          clonePresets(c.Presets),
	}
}

func (c Config) ManualSpeedPtr() *int {
	if !c.ManualEnabled {
		return nil
	}
	v := c.ManualSpeed
	return &v
}

func (c Config) TargetConfig() controller.TargetConfig {
	return controller.TargetConfig{
		Temp:       c.TargetTemp,
		Hysteresis: c.TargetHysteresis,
		KP:         c.TargetKP,
		Bias:       c.TargetBias,
		Min:        c.TargetMin,
		Max:        c.TargetMax,
		UpStep:     c.TargetUpStep,
		DownStep:   c.TargetDownStep,
	}
}

func ApplyPreset(cfg *Config, key string) bool {
	preset, ok := cfg.Presets[key]
	if !ok {
		return false
	}
	cfg.ActivePreset = key
	cfg.Curve = cloneCurve(preset.Curve)
	cfg.Strategy = preset.Strategy
	// 预设是曲线快照；应用预设时切回曲线模式，避免在目标温度模式下“无效果”。
	cfg.ControlMode = controller.ControlModeCurve
	return true
}

// SavePreset 把当前工作状态（曲线 + 策略）写回指定挡位槽。
// 用于「保存」按钮：编辑后显式持久化到该挡位。
func SavePreset(cfg *Config, key string) bool {
	existing, ok := cfg.Presets[key]
	if !ok {
		return false
	}
	cfg.Presets[key] = PresetConfig{Label: existing.Label, Curve: cloneCurve(cfg.Curve), Strategy: cfg.Strategy}
	cfg.ActivePreset = key
	return true
}

// MaxPresets 限制挡位总数（含内置），防止自定义挡位无限增长撑大配置文件。
// MaxPresetLabel 限制挡位名称长度（按 rune 计），超出则截断。
const (
	MaxPresets     = 24
	MaxPresetLabel = 40
)

// AddPreset 用当前工作状态新建一个自定义挡位并切换过去。
// label 为显示名称；key 由调用方生成且必须唯一、非内置。
func AddPreset(cfg *Config, key, label string) bool {
	if key == "" || IsBuiltinPreset(key) {
		return false
	}
	if _, exists := cfg.Presets[key]; exists {
		return false
	}
	if len(cfg.Presets) >= MaxPresets {
		return false
	}
	label = sanitizeLabel(label)
	if label == "" {
		label = key
	}
	cfg.Presets[key] = PresetConfig{Label: label, Curve: cloneCurve(cfg.Curve), Strategy: cfg.Strategy}
	cfg.ActivePreset = key
	return true
}

// sanitizeLabel 去除首尾空白并按 rune 截断到 MaxPresetLabel，避免超长名称。
func sanitizeLabel(label string) string {
	label = strings.TrimSpace(label)
	runes := []rune(label)
	if len(runes) > MaxPresetLabel {
		runes = runes[:MaxPresetLabel]
	}
	return string(runes)
}

// RestorePreset 把内置挡位恢复为出厂参数。自定义挡位不可恢复（返回 false）。
// 若恢复的是当前激活挡位，工作状态同步刷新为出厂值。
func RestorePreset(cfg *Config, key string) bool {
	def, ok := DefaultPresetConfig(key)
	if !ok {
		return false
	}
	cfg.Presets[key] = def
	if cfg.ActivePreset == key {
		cfg.Curve = cloneCurve(def.Curve)
		cfg.Strategy = def.Strategy
		cfg.ControlMode = controller.ControlModeCurve
	}
	return true
}

// DeletePreset 删除自定义挡位。内置挡位不可删除（返回 false）。
// 若删除的是当前激活挡位，则回退到「平衡」。
func DeletePreset(cfg *Config, key string) bool {
	if IsBuiltinPreset(key) {
		return false
	}
	if _, ok := cfg.Presets[key]; !ok {
		return false
	}
	delete(cfg.Presets, key)
	if cfg.ActivePreset == key {
		ApplyPreset(cfg, "balanced")
	}
	return true
}

func normalizeCurve(raw []controller.Point) []controller.Point {
	if len(raw) != controller.CurvePoints {
		return cloneCurve(controller.DefaultCurve)
	}
	points := make([]controller.Point, 0, len(raw))
	for _, point := range raw {
		if math.IsNaN(point.Temp()) || math.IsInf(point.Temp(), 0) || math.IsNaN(point.Speed()) || math.IsInf(point.Speed(), 0) {
			return cloneCurve(controller.DefaultCurve)
		}
		temp := math.Round(clamp(point.Temp(), controller.CurveTempMin, controller.CurveTempMax)*10) / 10
		speed := math.Round(clamp(point.Speed(), controller.CurveSpeedMin, controller.CurveSpeedMax)*10) / 10
		points = append(points, controller.Point{temp, speed})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Temp() < points[j].Temp() })
	for i := 1; i < len(points); i++ {
		if points[i].Temp() <= points[i-1].Temp() {
			temp := math.Round(math.Min(controller.CurveTempMax, points[i-1].Temp()+0.5)*10) / 10
			if temp <= points[i-1].Temp() {
				return cloneCurve(controller.DefaultCurve)
			}
			points[i][0] = temp
		}
	}
	return points
}

// builtinPresetKeys 返回内置挡位的 key 集合，用于区分内置与自定义挡位。
func builtinPresetKeys() map[string]bool {
	keys := make(map[string]bool, len(controller.Presets))
	for _, preset := range controller.Presets {
		keys[preset.Key] = true
	}
	return keys
}

// IsBuiltinPreset 报告 key 是否为内置挡位（静音/平衡/性能）。
func IsBuiltinPreset(key string) bool {
	return builtinPresetKeys()[key]
}

// DefaultPresetConfig 返回内置挡位的出厂参数，用于「恢复默认」。
func DefaultPresetConfig(key string) (PresetConfig, bool) {
	for _, preset := range controller.Presets {
		if preset.Key == key {
			return PresetConfig{Label: preset.Label, Curve: cloneCurve(preset.Curve), Strategy: preset.Strategy}, true
		}
	}
	return PresetConfig{}, false
}

func normalizePresets(raw map[string]PresetConfig) map[string]PresetConfig {
	builtins := builtinPresetKeys()
	defaults := defaultPresets()
	out := make(map[string]PresetConfig, len(raw)+len(defaults))
	// 保留所有挡位（含用户自定义），逐个规范化曲线/策略/名称。
	for key, preset := range raw {
		if key == "" {
			continue
		}
		fallbackStrategy := controller.DefaultStrategy
		label := preset.Label
		if def, ok := defaults[key]; ok {
			fallbackStrategy = def.Strategy
			if label == "" {
				label = def.Label
			}
		}
		label = sanitizeLabel(label)
		if label == "" {
			label = key
		}
		out[key] = PresetConfig{
			Label:    label,
			Curve:    normalizeCurve(preset.Curve),
			Strategy: normalizeStrategy(preset.Strategy, fallbackStrategy),
		}
	}
	// 内置挡位始终存在：缺失则补回出厂默认。
	for key := range builtins {
		if _, ok := out[key]; !ok {
			out[key] = defaults[key]
		}
	}
	return out
}

func normalizeStrategy(value string, fallback string) string {
	if controller.ValidStrategy(value) {
		return value
	}
	return fallback
}

func defaultPresets() map[string]PresetConfig {
	presets := make(map[string]PresetConfig, len(controller.Presets))
	for _, preset := range controller.Presets {
		presets[preset.Key] = PresetConfig{Label: preset.Label, Curve: cloneCurve(preset.Curve), Strategy: preset.Strategy}
	}
	return presets
}

func cloneCurve(curve []controller.Point) []controller.Point {
	out := make([]controller.Point, len(curve))
	copy(out, curve)
	return out
}

func clonePresets(presets map[string]PresetConfig) map[string]PresetConfig {
	out := make(map[string]PresetConfig, len(presets))
	for key, preset := range presets {
		out[key] = PresetConfig{Label: preset.Label, Curve: cloneCurve(preset.Curve), Strategy: preset.Strategy}
	}
	return out
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func makeLegacyCurve(lowT, lowS, maxT, maxS int) []controller.Point {
	curve := make([]controller.Point, controller.CurvePoints)
	for i := 0; i < controller.CurvePoints; i++ {
		ratio := float64(i) / float64(controller.CurvePoints-1)
		curve[i] = controller.Point{float64(lowT) + float64(maxT-lowT)*ratio, float64(lowS) + float64(maxS-lowS)*ratio}
	}
	return curve
}
