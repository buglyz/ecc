import json
import logging
import os

from .constants import (
    DEFAULT_CONFIG,
    CURVE_POINTS,
    CURVE_TEMP_MAX,
    CURVE_TEMP_MIN,
    CURVE_SPEED_MAX,
    CURVE_SPEED_MIN,
    DEFAULT_STRATEGY,
    DEFAULT_CURVE,
    PRESETS,
    STRATEGIES,
)
from .paths import APP_LEGACY_CONFIG_PATHS, CONFIG_PATH, LEGACY_CONFIG_PATH, STATE_DIR


def _clamp(value, minimum, maximum):
    return max(minimum, min(maximum, value))


def _normalize_curve(raw_curve):
    if not isinstance(raw_curve, list):
        return [list(p) for p in DEFAULT_CURVE]
    points = []
    for raw in raw_curve:
        if not isinstance(raw, (list, tuple)) or len(raw) != 2:
            continue
        try:
            temp = float(raw[0])
            speed = float(raw[1])
        except (TypeError, ValueError):
            continue
        points.append([
            round(_clamp(temp, CURVE_TEMP_MIN, CURVE_TEMP_MAX), 1),
            round(_clamp(speed, CURVE_SPEED_MIN, CURVE_SPEED_MAX), 1),
        ])
    if len(points) != CURVE_POINTS:
        return [list(p) for p in DEFAULT_CURVE]
    points.sort(key=lambda p: p[0])
    for i in range(1, len(points)):
        if points[i][0] <= points[i - 1][0]:
            points[i][0] = round(min(CURVE_TEMP_MAX, points[i - 1][0] + 0.5), 1)
    return points


def _normalize_presets(raw_presets):
    presets = {
        key: {"curve": [list(p) for p in curve], "strategy": strategy}
        for key, _label, curve, strategy in PRESETS
    }
    if not isinstance(raw_presets, dict):
        return presets
    valid_strategies = {k for k, _label in STRATEGIES}
    for key in presets:
        raw = raw_presets.get(key)
        if not isinstance(raw, dict):
            continue
        presets[key]["curve"] = _normalize_curve(raw.get("curve"))
        strategy = raw.get("strategy")
        if strategy in valid_strategies:
            presets[key]["strategy"] = strategy
    return presets


def _normalize_config(cfg):
    normalized = dict(DEFAULT_CONFIG)
    normalized.update({k: v for k, v in cfg.items() if k in DEFAULT_CONFIG})
    normalized["curve"] = _normalize_curve(normalized.get("curve"))
    normalized["presets"] = _normalize_presets(normalized.get("presets"))
    if normalized.get("active_preset") not in normalized["presets"]:
        normalized["active_preset"] = "balanced"
    if normalized.get("strategy") not in {k for k, _label in STRATEGIES}:
        normalized["strategy"] = DEFAULT_STRATEGY
    if normalized.get("theme") not in ("light", "dark"):
        normalized["theme"] = "light"
    try:
        normalized["manual_speed"] = int(_clamp(int(normalized.get("manual_speed", 50)), 0, 100))
    except (TypeError, ValueError):
        normalized["manual_speed"] = 50
    normalized["manual_enabled"] = bool(normalized.get("manual_enabled"))
    normalized["minimize"] = 1 if normalized.get("minimize") == 1 else 0
    try:
        minutes = int(normalized.get("time_entry", "5"))
        normalized["time_entry"] = str(_clamp(minutes, 1, 480))
    except (TypeError, ValueError):
        normalized["time_entry"] = "5"
    return normalized


def migrate_legacy(raw):
    if not isinstance(raw, dict):
        return _normalize_config({})
    cfg = dict(DEFAULT_CONFIG)
    cfg["presets"] = _normalize_presets(raw.get("presets"))
    cfg.update({k: v for k, v in raw.items() if k in DEFAULT_CONFIG and k != "presets"})
    if cfg.get("active_preset") not in cfg["presets"]:
        cfg["active_preset"] = "balanced"
    if "curve" not in raw and all(k in raw for k in ("low_t", "low_s", "max_t", "max_s")):
        try:
            lt, ls = int(raw["low_t"]), int(raw["low_s"])
            mt, ms = int(raw["max_t"]), int(raw["max_s"])
            cfg["curve"] = [
                [lt + (mt - lt) * i / (CURVE_POINTS - 1),
                 ls + (ms - ls) * i / (CURVE_POINTS - 1)]
                for i in range(CURVE_POINTS)
            ]
        except (TypeError, ValueError):
            pass
    return _normalize_config(cfg)


def load_config():
    config_candidates = (CONFIG_PATH, APP_LEGACY_CONFIG_PATHS[0])
    for path in config_candidates:
        if os.path.exists(path):
            try:
                with open(path, 'r', encoding='utf-8') as f:
                    raw = json.load(f)
                return migrate_legacy(raw)
            except (OSError, json.JSONDecodeError):
                logging.exception("配置读取失败: %s", path)
                return dict(DEFAULT_CONFIG)

    legacy_candidates = (LEGACY_CONFIG_PATH, APP_LEGACY_CONFIG_PATHS[1])
    for path in legacy_candidates:
        if os.path.exists(path):
            try:
                import pickle
                with open(path, 'rb') as f:
                    return migrate_legacy(pickle.load(f))
            except Exception:
                logging.exception("旧配置读取失败: %s", path)
    return dict(DEFAULT_CONFIG)


def save_config(cfg):
    temp_path = CONFIG_PATH + '.tmp'
    try:
        os.makedirs(STATE_DIR, exist_ok=True)
        with open(temp_path, 'w', encoding='utf-8') as f:
            json.dump(cfg, f, ensure_ascii=False, indent=2)
        os.replace(temp_path, CONFIG_PATH)
    except OSError:
        logging.exception("配置保存失败: %s", CONFIG_PATH)
        try:
            if os.path.exists(temp_path):
                os.remove(temp_path)
        except OSError:
            pass
