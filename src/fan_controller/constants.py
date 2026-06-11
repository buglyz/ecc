APP_NAME = 'FanController'

EC_REG_FAN1 = '0x2C'
EC_REG_FAN2 = '0x2D'
EC_FAN_RELEASE = '0xFF'
SAMPLES_PER_CYCLE = 6
SAMPLE_INTERVAL = 1.0
HYSTERESIS_TEMP = 2.0
LOOP_DRIFT_TOLERANCE = 5.0
EXPECTED_CYCLE_DURATION = SAMPLES_PER_CYCLE * SAMPLE_INTERVAL + 1.3
HEARTBEAT_SECONDS = 30.0
HISTORY_MAX_SAMPLES = 28800
STATUS_REFRESH_MS = 1000
PLOT_REFRESH_MS = 5000

CURVE_POINTS = 5
CURVE_TEMP_MIN = 30
CURVE_TEMP_MAX = 100
CURVE_SPEED_MIN = 0
CURVE_SPEED_MAX = 100
DEFAULT_CURVE = [[40, 30], [55, 40], [70, 60], [80, 85], [90, 100]]

STRATEGIES = [
    ("weighted", "加权 (0.7·CPU + 0.3·GPU)"),
    ("max", "取最大值 max(CPU, GPU)"),
    ("cpu", "仅 CPU"),
    ("gpu", "仅 GPU"),
]
DEFAULT_STRATEGY = "weighted"
CPU_WEIGHT = 0.7

DEFAULT_PRESETS = [
    ("silent", "静音", [[40, 15], [55, 25], [70, 40], [80, 60], [90, 85]], "weighted"),
    ("balanced", "平衡", [[40, 30], [55, 40], [70, 60], [80, 85], [90, 100]], "weighted"),
    ("performance", "性能", [[40, 40], [55, 60], [65, 80], [75, 95], [85, 100]], "max"),
]
PRESETS = DEFAULT_PRESETS
STARTUP_IDENTIFIER = '风扇控制'


def default_presets_config():
    return {
        key: {"curve": [list(p) for p in curve], "strategy": strategy}
        for key, _label, curve, strategy in DEFAULT_PRESETS
    }


DEFAULT_CONFIG = {
    "curve": DEFAULT_CURVE,
    "strategy": DEFAULT_STRATEGY,
    "theme": "light",
    "time_entry": "5",
    "minimize": 0,
    "manual_enabled": False,
    "manual_speed": 50,
    "active_preset": "balanced",
    "presets": default_presets_config(),
}
