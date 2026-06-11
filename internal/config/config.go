package config

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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

func legacyCurveFromValues(values map[string]any) ([]controller.Point, bool) {
	lowT, ok := intFromAny(values["low_t"])
	if !ok {
		return nil, false
	}
	lowS, ok := intFromAny(values["low_s"])
	if !ok {
		return nil, false
	}
	maxT, ok := intFromAny(values["max_t"])
	if !ok {
		return nil, false
	}
	maxS, ok := intFromAny(values["max_s"])
	if !ok {
		return nil, false
	}
	return makeLegacyCurve(lowT, lowS, maxT, maxS), true
}

func curveFromAny(value any) ([]controller.Point, bool) {
	items, ok := value.([]any)
	if !ok || len(items) != controller.CurvePoints {
		return nil, false
	}
	curve := make([]controller.Point, 0, len(items))
	for _, item := range items {
		pair, ok := item.([]any)
		if !ok || len(pair) != 2 {
			return nil, false
		}
		temp, ok := floatFromAny(pair[0])
		if !ok {
			return nil, false
		}
		speed, ok := floatFromAny(pair[1])
		if !ok {
			return nil, false
		}
		curve = append(curve, controller.Point{temp, speed})
	}
	return curve, true
}

func presetsFromAny(value any) (map[string]PresetConfig, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	presets := make(map[string]PresetConfig, len(raw))
	for key, item := range raw {
		presetMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		curve, ok := curveFromAny(presetMap["curve"])
		if !ok {
			continue
		}
		strategy, ok := stringFromAny(presetMap["strategy"])
		if !ok {
			strategy = controller.DefaultStrategy
		}
		presets[key] = PresetConfig{Curve: curve, Strategy: strategy}
	}
	return presets, len(presets) > 0
}

func boolFromAny(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	}
	return false, false
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(v)
		return n, err == nil
	}
	return 0, false
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case string:
		n, err := strconv.ParseFloat(v, 64)
		return n, err == nil
	}
	return 0, false
}

func stringFromAny(value any) (string, bool) {
	v, ok := value.(string)
	return v, ok
}

type pickleParser struct {
	data  []byte
	pos   int
	stack []any
	marks []int
	memo  map[int]any
}

func parsePickleDict(data []byte) (out map[string]any, err error) {
	// 截断/损坏的 pickle 可能让某些 opcode 处理器读越界（readByte/stack 切片）。
	// 这里兜底 recover，把 panic 转成普通 error，避免启动时 config.Load 崩溃整程序。
	defer func() {
		if r := recover(); r != nil {
			out = nil
			err = fmt.Errorf("pickle parse panic: %v", r)
		}
	}()
	p := &pickleParser{data: data, memo: map[int]any{}}
	value, err := p.parse()
	if err != nil {
		return nil, err
	}
	out, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("pickle root is not dict")
	}
	return out, nil
}

func (p *pickleParser) parse() (any, error) {
	for p.pos < len(p.data) {
		op := p.readByte()
		switch op {
		case 0x80:
			p.pos++
		case 0x95:
			if p.pos+8 > len(p.data) {
				return nil, errors.New("short FRAME")
			}
			p.pos += 8
		case 0x94:
			if len(p.stack) > 0 {
				p.memo[len(p.memo)] = p.stack[len(p.stack)-1]
			}
		case '}':
			p.stack = append(p.stack, map[string]any{})
		case ']':
			p.stack = append(p.stack, []any{})
		case '(':
			p.marks = append(p.marks, len(p.stack))
		case 'e':
			if err := p.appendItems(); err != nil {
				return nil, err
			}
		case 'u':
			if err := p.setItems(); err != nil {
				return nil, err
			}
		case 's':
			if err := p.setItem(); err != nil {
				return nil, err
			}
		case 'K':
			p.stack = append(p.stack, int(p.readByte()))
		case 'J':
			if p.pos+4 > len(p.data) {
				return nil, errors.New("short BININT")
			}
			value := int(int32(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4])))
			p.pos += 4
			p.stack = append(p.stack, value)
		case 'M':
			if p.pos+2 > len(p.data) {
				return nil, errors.New("short BININT2")
			}
			value := int(binary.LittleEndian.Uint16(p.data[p.pos : p.pos+2]))
			p.pos += 2
			p.stack = append(p.stack, value)
		case 'G':
			if p.pos+8 > len(p.data) {
				return nil, errors.New("short BINFLOAT")
			}
			bits := binary.BigEndian.Uint64(p.data[p.pos : p.pos+8])
			p.pos += 8
			p.stack = append(p.stack, math.Float64frombits(bits))
		case 'N':
			p.stack = append(p.stack, nil)
		case ')':
			p.stack = append(p.stack, []any{})
		case 0x85:
			if len(p.stack) < 1 {
				return nil, errors.New("TUPLE1 needs one item")
			}
			item := p.stack[len(p.stack)-1]
			p.stack[len(p.stack)-1] = []any{item}
		case 0x86:
			if len(p.stack) < 2 {
				return nil, errors.New("TUPLE2 needs two items")
			}
			items := []any{p.stack[len(p.stack)-2], p.stack[len(p.stack)-1]}
			p.stack = append(p.stack[:len(p.stack)-2], items)
		case 0x87:
			if len(p.stack) < 3 {
				return nil, errors.New("TUPLE3 needs three items")
			}
			items := []any{p.stack[len(p.stack)-3], p.stack[len(p.stack)-2], p.stack[len(p.stack)-1]}
			p.stack = append(p.stack[:len(p.stack)-3], items)
		case 't':
			if err := p.makeTuple(); err != nil {
				return nil, err
			}
		case 0x88:
			p.stack = append(p.stack, true)
		case 0x89:
			p.stack = append(p.stack, false)
		case 0x8c:
			text, err := p.readShortString()
			if err != nil {
				return nil, err
			}
			p.stack = append(p.stack, text)
		case 'X':
			text, err := p.readBinUnicode()
			if err != nil {
				return nil, err
			}
			p.stack = append(p.stack, text)
		case 'q':
			idx := int(p.readByte())
			if len(p.stack) > 0 {
				p.memo[idx] = p.stack[len(p.stack)-1]
			}
		case 'r':
			if p.pos+4 > len(p.data) {
				return nil, errors.New("short LONG_BINPUT")
			}
			idx := int(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4]))
			p.pos += 4
			if len(p.stack) > 0 {
				p.memo[idx] = p.stack[len(p.stack)-1]
			}
		case 'h':
			idx := int(p.readByte())
			p.stack = append(p.stack, p.memo[idx])
		case 'j':
			if p.pos+4 > len(p.data) {
				return nil, errors.New("short LONG_BINGET")
			}
			idx := int(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4]))
			p.pos += 4
			p.stack = append(p.stack, p.memo[idx])
		case '.':
			if len(p.stack) == 0 {
				return nil, errors.New("empty pickle stack")
			}
			return p.stack[len(p.stack)-1], nil
		default:
			return nil, fmt.Errorf("unsupported pickle opcode 0x%x", op)
		}
	}
	return nil, errors.New("pickle missing STOP")
}

func (p *pickleParser) makeTuple() error {
	mark, err := p.popMark()
	if err != nil {
		return err
	}
	items := append([]any(nil), p.stack[mark:]...)
	p.stack = append(p.stack[:mark], items)
	return nil
}

func (p *pickleParser) appendItems() error {
	mark, err := p.popMark()
	if err != nil {
		return err
	}
	if mark == 0 {
		return errors.New("APPENDS missing list")
	}
	items := append([]any(nil), p.stack[mark:]...)
	list, ok := p.stack[mark-1].([]any)
	if !ok {
		return errors.New("APPENDS target is not list")
	}
	list = append(list, items...)
	p.stack = append(p.stack[:mark-1], list)
	return nil
}

func (p *pickleParser) setItems() error {
	mark, err := p.popMark()
	if err != nil {
		return err
	}
	if mark == 0 {
		return errors.New("SETITEMS missing dict")
	}
	dict, ok := p.stack[mark-1].(map[string]any)
	if !ok {
		return errors.New("SETITEMS target is not dict")
	}
	items := p.stack[mark:]
	if len(items)%2 != 0 {
		return errors.New("SETITEMS needs key/value pairs")
	}
	for i := 0; i < len(items); i += 2 {
		key, ok := items[i].(string)
		if !ok {
			return errors.New("SETITEMS key is not string")
		}
		dict[key] = items[i+1]
	}
	p.stack = append(p.stack[:mark-1], dict)
	return nil
}

func (p *pickleParser) setItem() error {
	if len(p.stack) < 3 {
		return errors.New("SETITEM needs dict/key/value")
	}
	value := p.stack[len(p.stack)-1]
	key, ok := p.stack[len(p.stack)-2].(string)
	if !ok {
		return errors.New("SETITEM key is not string")
	}
	dict, ok := p.stack[len(p.stack)-3].(map[string]any)
	if !ok {
		return errors.New("SETITEM target is not dict")
	}
	dict[key] = value
	p.stack = p.stack[:len(p.stack)-2]
	p.stack[len(p.stack)-1] = dict
	return nil
}

func (p *pickleParser) popMark() (int, error) {
	if len(p.marks) == 0 {
		return 0, errors.New("missing mark")
	}
	mark := p.marks[len(p.marks)-1]
	p.marks = p.marks[:len(p.marks)-1]
	if mark < 0 || mark > len(p.stack) {
		return 0, errors.New("mark out of range")
	}
	return mark, nil
}

func (p *pickleParser) readShortString() (string, error) {
	length := int(p.readByte())
	if p.pos+length > len(p.data) {
		return "", errors.New("short SHORT_BINUNICODE")
	}
	text := string(p.data[p.pos : p.pos+length])
	p.pos += length
	return text, nil
}

func (p *pickleParser) readBinUnicode() (string, error) {
	if p.pos+4 > len(p.data) {
		return "", errors.New("short BINUNICODE")
	}
	length := int(binary.LittleEndian.Uint32(p.data[p.pos : p.pos+4]))
	p.pos += 4
	if length < 0 || p.pos+length > len(p.data) || bytes.IndexByte(p.data[p.pos:p.pos+length], 0) >= 0 {
		return "", errors.New("invalid BINUNICODE")
	}
	text := string(p.data[p.pos : p.pos+length])
	p.pos += length
	return text, nil
}

func (p *pickleParser) readByte() byte {
	if p.pos >= len(p.data) {
		// 越界时安全返回 0 并仍推进 pos，使主循环 p.pos<len(p.data) 自然终止，
		// 避免截断/损坏数据触发 index out of range panic（recover 也是兜底）。
		p.pos++
		return 0
	}
	value := p.data[p.pos]
	p.pos++
	return value
}
