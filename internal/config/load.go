package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

// LoadResult 包含配置加载的结果和可能的错误信息
type LoadResult struct {
	Config         Config
	CorruptedFiles []string // 损坏的配置文件路径（已备份）
}

// Load 加载配置文件，优先尝试 JSON，回退到 pickle。
// 如果主配置损坏，会备份为 .corrupted.{timestamp} 并返回默认配置。
func Load(p paths.Paths) LoadResult {
	result := LoadResult{Config: Default()}

	// 尝试 JSON 候选
	for _, candidate := range jsonCandidates(p) {
		if _, err := os.Stat(candidate); err == nil {
			if cfg, ok := loadJSON(candidate); ok {
				result.Config = Normalize(cfg)
				return result
			}
			// JSON 主配置存在但损坏：备份并标记
			backupCorruptedConfig(candidate)
			result.CorruptedFiles = append(result.CorruptedFiles, candidate)
			// 主 config 损坏不应静默回落默认并丢弃可迁移的 legacy pickle 数据：
			// 跳出 JSON 候选、继续尝试 pickle 候选，全部失败才回落 Default()。
			break
		}
	}

	// 尝试 pickle 候选
	for _, candidate := range pickleCandidates(p) {
		if _, err := os.Stat(candidate); err == nil {
			if cfg, ok := loadPickle(candidate); ok {
				result.Config = Normalize(cfg)
				return result
			}
			// pickle 存在但损坏：备份并标记
			backupCorruptedConfig(candidate)
			result.CorruptedFiles = append(result.CorruptedFiles, candidate)
		}
	}

	// 全部失败，返回默认配置
	return result
}

// backupCorruptedConfig 将损坏的配置文件备份为 .corrupted.{timestamp}
func backupCorruptedConfig(path string) {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.corrupted.%s", path, timestamp)
	if err := os.Rename(path, backupPath); err != nil {
		// 备份失败也不阻止启动，但记录到标准错误
		log.Printf("配置备份失败: %s -> %s: %v", path, backupPath, err)
	}
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
