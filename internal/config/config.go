package config

import (
	"encoding/json"
	"errors"
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
	Curve         []controller.Point      `json:"curve"`
	Strategy      string                  `json:"strategy"`
	Theme         string                  `json:"theme"`
	TimeEntry     string                  `json:"time_entry"`
	Minimize      int                     `json:"minimize"`
	ManualEnabled bool                    `json:"manual_enabled"`
	ManualSpeed   int                     `json:"manual_speed"`
	ActivePreset  string                  `json:"active_preset"`
	Presets       map[string]PresetConfig `json:"presets"`
}

func Default() Config {
	return Config{
		Curve:         cloneCurve(controller.DefaultCurve),
		Strategy:      controller.DefaultStrategy,
		Theme:         "light",
		TimeEntry:     "5",
		Minimize:      0,
		ManualEnabled: false,
		ManualSpeed:   50,
		ActivePreset:  "balanced",
		Presets:       defaultPresets(),
	}
}

func Load(p paths.Paths) Config {
	for _, candidate := range jsonCandidates(p) {
		if _, err := os.Stat(candidate); err == nil {
			if cfg, ok := loadJSON(candidate); ok {
				return Normalize(cfg)
			}
			// 主 config 损坏不应静默回落默认并丢弃可迁移的 legacy pickle 数据：
			// 跳出 JSON 候选、继续尝试 pickle 候选，全部失败才回落 Default()。
			break
		}
	}
	for _, candidate := range pickleCandidates(p) {
		if cfg, ok := loadPickle(candidate); ok {
			return Normalize(cfg)
		}
	}
	return Default()
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
		Curve:         cloneCurve(c.Curve),
		Strategy:      c.Strategy,
		Theme:         c.Theme,
		TimeEntry:     c.TimeEntry,
		Minimize:      c.Minimize,
		ManualEnabled: c.ManualEnabled,
		ManualSpeed:   c.ManualSpeed,
		ActivePreset:  c.ActivePreset,
		Presets:       clonePresets(c.Presets),
	}
}

func (c Config) ManualSpeedPtr() *int {
	if !c.ManualEnabled {
		return nil
	}
	v := c.ManualSpeed
	return &v
}

func ApplyPreset(cfg *Config, key string) bool {
	preset, ok := cfg.Presets[key]
	if !ok {
		return false
	}
	cfg.ActivePreset = key
	cfg.Curve = cloneCurve(preset.Curve)
	cfg.Strategy = preset.Strategy
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

func jsonCandidates(p paths.Paths) []string {
	legacy := paths.AppLegacyConfig(p)
	return []string{p.ConfigPath, legacy[0]}
}

func pickleCandidates(p paths.Paths) []string {
	legacy := paths.AppLegacyConfig(p)
	return []string{p.LegacyData, legacy[1]}
}

func loadJSON(path string) (Config, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, false
	}
	cfg := Default()
	if value, ok := raw["curve"]; ok {
		var curve []controller.Point
		if err := json.Unmarshal(value, &curve); err == nil {
			cfg.Curve = curve
		}
	}
	if value, ok := raw["strategy"]; ok {
		_ = json.Unmarshal(value, &cfg.Strategy)
	}
	if value, ok := raw["theme"]; ok {
		_ = json.Unmarshal(value, &cfg.Theme)
	}
	if value, ok := raw["time_entry"]; ok {
		var timeStr string
		if json.Unmarshal(value, &timeStr) == nil {
			cfg.TimeEntry = timeStr
		} else if parsed, err := rawIntValue(value); err == nil {
			cfg.TimeEntry = strconv.Itoa(parsed)
		}
	}
	if value, ok := raw["minimize"]; ok {
		if parsed, err := rawIntValue(value); err == nil {
			cfg.Minimize = parsed
		} else {
			var b bool
			if json.Unmarshal(value, &b) == nil && b {
				cfg.Minimize = 1
			}
		}
	}
	if value, ok := raw["manual_enabled"]; ok {
		var b bool
		if json.Unmarshal(value, &b) == nil {
			cfg.ManualEnabled = b
		} else if parsed, err := rawIntValue(value); err == nil {
			cfg.ManualEnabled = parsed != 0
		}
	}
	if value, ok := raw["manual_speed"]; ok {
		if parsed, err := rawIntValue(value); err == nil {
			cfg.ManualSpeed = parsed
		}
	}
	if value, ok := raw["active_preset"]; ok {
		_ = json.Unmarshal(value, &cfg.ActivePreset)
	}
	if value, ok := raw["presets"]; ok {
		var presets map[string]PresetConfig
		if err := json.Unmarshal(value, &presets); err == nil {
			cfg.Presets = presets
		}
	}
	if _, ok := raw["curve"]; !ok {
		legacyCurve, err := legacyCurve(raw)
		if err == nil {
			cfg.Curve = legacyCurve
		}
	}
	return cfg, true
}

func loadPickle(path string) (Config, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false
	}
	values, err := parsePickleDict(data)
	if err != nil {
		return Config{}, false
	}
	return configFromValues(values), true
}

func configFromValues(values map[string]any) Config {
	cfg := Default()
	if curve, ok := curveFromAny(values["curve"]); ok {
		cfg.Curve = curve
	} else if curve, ok := legacyCurveFromValues(values); ok {
		cfg.Curve = curve
	}
	if v, ok := stringFromAny(values["strategy"]); ok {
		cfg.Strategy = v
	}
	if v, ok := stringFromAny(values["theme"]); ok {
		cfg.Theme = v
	}
	if v, ok := stringFromAny(values["time_entry"]); ok {
		cfg.TimeEntry = v
	} else if v, ok := intFromAny(values["time_entry"]); ok {
		cfg.TimeEntry = strconv.Itoa(v)
	}
	if v, ok := intFromAny(values["minimize"]); ok {
		cfg.Minimize = v
	}
	if v, ok := boolFromAny(values["manual_enabled"]); ok {
		cfg.ManualEnabled = v
	}
	if v, ok := intFromAny(values["manual_speed"]); ok {
		cfg.ManualSpeed = v
	}
	if v, ok := stringFromAny(values["active_preset"]); ok {
		cfg.ActivePreset = v
	}
	if presets, ok := presetsFromAny(values["presets"]); ok {
		cfg.Presets = presets
	}
	return cfg
}

func legacyCurve(raw map[string]json.RawMessage) ([]controller.Point, error) {
	lowT, err := rawInt(raw, "low_t")
	if err != nil {
		return nil, err
	}
	lowS, err := rawInt(raw, "low_s")
	if err != nil {
		return nil, err
	}
	maxT, err := rawInt(raw, "max_t")
	if err != nil {
		return nil, err
	}
	maxS, err := rawInt(raw, "max_s")
	if err != nil {
		return nil, err
	}
	return makeLegacyCurve(lowT, lowS, maxT, maxS), nil
}

func rawInt(raw map[string]json.RawMessage, key string) (int, error) {
	value, ok := raw[key]
	if !ok {
		return 0, errors.New("missing legacy key")
	}
	return rawIntValue(value)
}

func rawIntValue(value json.RawMessage) (int, error) {
	var number int
	if err := json.Unmarshal(value, &number); err == nil {
		return number, nil
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return 0, err
	}
	return strconv.Atoi(text)
}

func normalizeCurve(raw []controller.Point) []controller.Point {
	if len(raw) != controller.CurvePoints {
		return cloneCurve(controller.DefaultCurve)
	}
	points := make([]controller.Point, 0, len(raw))
	for _, point := range raw {
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

