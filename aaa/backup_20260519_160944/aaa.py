import ctypes
import datetime
import json
import os
import subprocess
import sys
import time
import tkinter as tk
from collections import deque
from threading import Event, Lock, Thread
from tkinter import messagebox, ttk

import clr
import matplotlib

matplotlib.use('Agg')  # before pyplot import, so pyplot doesn't spawn its own Tk root

import matplotlib.font_manager as fm
import matplotlib.pyplot as plt
import pystray
from PIL import Image, ImageDraw, ImageFont
from matplotlib.backends.backend_tkagg import FigureCanvasTkAgg

try:
    import sv_ttk
    HAS_SV_TTK = True
except ImportError:
    HAS_SV_TTK = False


# ---------- Paths ----------
if getattr(sys, 'frozen', False):
    APP_DIR = os.path.dirname(sys.executable)
else:
    APP_DIR = os.path.dirname(os.path.abspath(__file__))

# ec-probe.exe ships one level up from this script, alongside the installer root
ROOT_DIR = os.path.abspath(os.path.join(APP_DIR, os.pardir))
EC_PROBE = os.path.join(ROOT_DIR, 'ec-probe.exe')
HWMON_DLL = os.path.join(APP_DIR, 'data', 'LibreHardwareMonitorLib.dll')
CONFIG_PATH = os.path.join(APP_DIR, 'config.json')
LEGACY_CONFIG_PATH = os.path.join(APP_DIR, 'data.dat')
FONT_PATH = 'C:/Windows/Fonts/simhei.ttf'

# ---------- EC + tuning constants ----------
EC_REG_FAN1 = '0x2C'
EC_REG_FAN2 = '0x2D'
EC_FAN_RELEASE = '0xFF'        # written on stop so the firmware regains fan control
SAMPLES_PER_CYCLE = 6
SAMPLE_INTERVAL = 1.0
HYSTERESIS_TEMP = 2.0          # require this much temp drift before re-deciding speed
LOOP_DRIFT_TOLERANCE = 5.0
EXPECTED_CYCLE_DURATION = SAMPLES_PER_CYCLE * SAMPLE_INTERVAL + 1.3
HEARTBEAT_SECONDS = 30.0       # force EC rewrite after this long, even if speed is stable
HISTORY_MAX_SAMPLES = 28800    # ~8 hours at 1Hz sampling
STATUS_REFRESH_MS = 1000
PLOT_REFRESH_MS = 5000

# Curve constraints (UI shows speed as 0-100 to match the original "%" label).
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
CPU_WEIGHT = 0.7  # used by weighted strategy

PRESETS = [
    ("silent", "静音", [[40, 15], [55, 25], [70, 40], [80, 60], [90, 85]], "weighted"),
    ("balanced", "平衡", [[40, 30], [55, 40], [70, 60], [80, 85], [90, 100]], "weighted"),
    ("performance", "性能", [[40, 40], [55, 60], [65, 80], [75, 95], [85, 100]], "max"),
]

STARTUP_IDENTIFIER = '风扇控制'
DEFAULT_CONFIG = {
    "curve": DEFAULT_CURVE,
    "strategy": DEFAULT_STRATEGY,
    "theme": "light",
    "time_entry": "5",
    "minimize": 0,
    "manual_enabled": False,
    "manual_speed": 50,
}


# ---------- Admin check ----------
def is_admin():
    try:
        return ctypes.windll.shell32.IsUserAnAdmin()
    except Exception:
        return False


if not is_admin():
    ctypes.windll.shell32.ShellExecuteW(None, "runas", sys.executable, __file__, None, 1)
    sys.exit()

ctypes.windll.shcore.SetProcessDpiAwareness(1)


# ---------- Subprocess plumbing ----------
_startupinfo = subprocess.STARTUPINFO()
_startupinfo.dwFlags = subprocess.STARTF_USESHOWWINDOW
_startupinfo.wShowWindow = subprocess.SW_HIDE


def _ec_write(register, value_hex):
    try:
        subprocess.Popen(
            [EC_PROBE, 'write', '-v', register, value_hex],
            startupinfo=_startupinfo,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except OSError:
        pass


# ---------- Hardware monitor ----------
clr.AddReference(HWMON_DLL)
from LibreHardwareMonitor import Hardware  # noqa: E402


# ---------- Curve helpers ----------
def interpolate_curve(curve, temp):
    points = sorted(curve, key=lambda p: p[0])
    if temp <= points[0][0]:
        return points[0][1]
    if temp >= points[-1][0]:
        return points[-1][1]
    for i in range(len(points) - 1):
        t1, s1 = points[i]
        t2, s2 = points[i + 1]
        if t1 <= temp <= t2:
            if t2 == t1:
                return s2
            return s1 + (temp - t1) / (t2 - t1) * (s2 - s1)
    return points[-1][1]


def combine_temps(strategy, cpu, gpu):
    if cpu is None and gpu is None:
        return None
    if strategy == "cpu":
        return cpu if cpu is not None else gpu
    if strategy == "gpu":
        return gpu if gpu is not None else cpu
    if strategy == "max":
        if cpu is None:
            return gpu
        if gpu is None:
            return cpu
        return max(cpu, gpu)
    # weighted (default)
    if cpu is None:
        return gpu
    if gpu is None:
        return cpu
    return (cpu - gpu) * CPU_WEIGHT + gpu


class FanController:
    def __init__(self, curve, strategy):
        self._curve_lock = Lock()
        self._curve = list(curve)
        self._strategy = strategy
        self._manual_speed = None  # None = automatic mode

        self._computer = Hardware.Computer()
        self._computer.IsCpuEnabled = True
        self._computer.IsGpuEnabled = True
        self._computer.Open()
        self._hardware = list(self._computer.Hardware)

        self.history = deque(maxlen=HISTORY_MAX_SAMPLES)
        self._history_lock = Lock()
        self.latest = (None, None, None, None)  # (cpu, gpu, combined_t, speed)

        self._stop_event = Event()
        self._thread = Thread(target=self._run, daemon=True)
        self._thread.start()

    def set_curve(self, curve):
        with self._curve_lock:
            self._curve = [list(p) for p in curve]

    def set_strategy(self, strategy):
        with self._curve_lock:
            self._strategy = strategy

    def set_manual(self, speed):
        """speed=None enters automatic mode; otherwise locks fan to that value."""
        with self._curve_lock:
            self._manual_speed = None if speed is None else int(speed)

    def snapshot(self):
        with self._history_lock:
            return list(self.history)

    def stop(self):
        self._stop_event.set()
        _ec_write(EC_REG_FAN1, EC_FAN_RELEASE)
        _ec_write(EC_REG_FAN2, EC_FAN_RELEASE)

    def _read_temps(self):
        try:
            cpu_hw = self._hardware[0]
            cpu_hw.Update()
            cpu_temps = [s.Value for s in cpu_hw.Sensors
                         if s.SensorType == Hardware.SensorType.Temperature]

            gpu_hw = self._hardware[1]
            gpu_hw.Update()
            gpu_temps = [s.Value for s in gpu_hw.Sensors
                         if s.SensorType == Hardware.SensorType.Temperature]
        except Exception:
            return None, None

        if not cpu_temps or not gpu_temps:
            return None, None
        # legacy index choice: cpu[-2] often points at CPU package before per-core
        c = cpu_temps[-2] if len(cpu_temps) >= 2 else cpu_temps[0]
        g = gpu_temps[0]
        return c, g

    def _target_speed(self, t):
        with self._curve_lock:
            curve = list(self._curve)
        return interpolate_curve(curve, t)

    def _current_mode(self):
        with self._curve_lock:
            return self._manual_speed, self._strategy

    def _run(self):
        time.sleep(1)
        current_speed = int(DEFAULT_CURVE[0][1])
        _ec_write(EC_REG_FAN1, hex(current_speed))
        _ec_write(EC_REG_FAN2, hex(current_speed))

        cycle_start = time.time()
        last_write_ts = time.time()
        last_committed_temp = None

        while not self._stop_event.is_set():
            cycle_temps = []
            for _ in range(SAMPLES_PER_CYCLE):
                if self._stop_event.is_set():
                    return
                c, g = self._read_temps()
                manual, strategy = self._current_mode()
                t = combine_temps(strategy, c, g)
                if t is not None:
                    cycle_temps.append(t)
                self.latest = (c, g, t, current_speed)
                with self._history_lock:
                    self.history.append(
                        (datetime.datetime.now(), c, g, t, current_speed)
                    )
                time.sleep(SAMPLE_INTERVAL)

            drifted = abs(time.time() - cycle_start - EXPECTED_CYCLE_DURATION) > LOOP_DRIFT_TOLERANCE
            heartbeat_due = (time.time() - last_write_ts) >= HEARTBEAT_SECONDS
            manual, _ = self._current_mode()

            if manual is not None:
                target = manual
                last_committed_temp = None  # reset so auto-mode re-decides freshly on switch back
            elif cycle_temps:
                avg_t = sum(cycle_temps) / len(cycle_temps)
                temp_settled = (last_committed_temp is not None
                                and abs(avg_t - last_committed_temp) < HYSTERESIS_TEMP)
                if temp_settled and not (drifted or heartbeat_due):
                    cycle_start = time.time()
                    continue
                target = self._target_speed(avg_t)
                last_committed_temp = avg_t
            else:
                cycle_start = time.time()
                continue

            target = max(0, min(100, int(round(target))))
            if target != current_speed or drifted or heartbeat_due:
                current_speed = target
                speed_hex = hex(current_speed)
                _ec_write(EC_REG_FAN1, speed_hex)
                _ec_write(EC_REG_FAN2, speed_hex)
                last_write_ts = time.time()
            cycle_start = time.time()


# ---------- Startup item helpers ----------
def _vbs_path(identifier):
    startup_dir = os.path.join(
        os.getenv('APPDATA'), 'Microsoft', 'Windows', 'Start Menu', 'Programs', 'Startup'
    )
    return os.path.join(startup_dir, f'{identifier}.vbs')


def is_in_startup(identifier):
    return os.path.exists(_vbs_path(identifier))


def add_to_startup(target_path, identifier):
    try:
        path = _vbs_path(identifier)
        with open(path, 'w', encoding='utf-8') as f:
            f.write('Set WshShell = CreateObject("WScript.Shell")\n')
            f.write(f'WshShell.Run "{target_path}", 0\n')
        return True
    except OSError:
        return False


def remove_from_startup(identifier):
    try:
        path = _vbs_path(identifier)
        if os.path.exists(path):
            os.remove(path)
        return True
    except OSError:
        return False


# ---------- Config persistence (with legacy migration) ----------
def _migrate_legacy(raw):
    """Translate the old low_t/low_s/max_t/max_s schema into a 5-point curve."""
    cfg = dict(DEFAULT_CONFIG)
    cfg.update({k: v for k, v in raw.items() if k in DEFAULT_CONFIG})
    if "curve" not in raw and all(k in raw for k in ("low_t", "low_s", "max_t", "max_s")):
        try:
            lt, ls = int(raw["low_t"]), int(raw["low_s"])
            mt, ms = int(raw["max_t"]), int(raw["max_s"])
            # spread evenly across [lt, mt] so the curve editor has CURVE_POINTS handles
            cfg["curve"] = [
                [lt + (mt - lt) * i / (CURVE_POINTS - 1),
                 ls + (ms - ls) * i / (CURVE_POINTS - 1)]
                for i in range(CURVE_POINTS)
            ]
            cfg["curve"] = [[round(t, 1), round(s, 1)] for t, s in cfg["curve"]]
        except (TypeError, ValueError):
            pass
    return cfg


def load_config():
    if os.path.exists(CONFIG_PATH):
        try:
            with open(CONFIG_PATH, 'r', encoding='utf-8') as f:
                raw = json.load(f)
            return _migrate_legacy(raw)
        except (OSError, json.JSONDecodeError):
            return dict(DEFAULT_CONFIG)
    if os.path.exists(LEGACY_CONFIG_PATH):
        try:
            import pickle
            with open(LEGACY_CONFIG_PATH, 'rb') as f:
                return _migrate_legacy(pickle.load(f))
        except Exception:
            pass
    return dict(DEFAULT_CONFIG)


def save_config(cfg):
    try:
        with open(CONFIG_PATH, 'w', encoding='utf-8') as f:
            json.dump(cfg, f, ensure_ascii=False, indent=2)
    except OSError:
        pass


# ---------- Theme palettes for matplotlib ----------
DARK_PALETTE = {
    "bg": "#1c1c1c", "fg": "#e6e6e6", "grid": "#3a3a3a",
    "cpu": "#ff6b6b", "gpu": "#4dabf7", "t": "#ffd43b",
    "speed": "#51cf66", "curve": "#74c0fc", "point": "#74c0fc",
}
LIGHT_PALETTE = {
    "bg": "#ffffff", "fg": "#1a1a1a", "grid": "#d0d0d0",
    "cpu": "#e03131", "gpu": "#1971c2", "t": "#f08c00",
    "speed": "#2f9e44", "curve": "#1971c2", "point": "#1971c2",
}


def palette_for(theme):
    return DARK_PALETTE if theme == "dark" else LIGHT_PALETTE


# ---------- Widgets ----------
class StatusBar(ttk.Frame):
    """Top bar with four large numeric readouts."""

    def __init__(self, master, font_family="Segoe UI"):
        super().__init__(master, padding=(10, 10))
        self._tiles = {}
        labels = [("cpu", "CPU"), ("gpu", "GPU"), ("t", "加权温度"), ("speed", "风扇")]
        for i, (key, label) in enumerate(labels):
            tile = ttk.Frame(self, padding=10, relief="flat")
            tile.grid(row=0, column=i, padx=8, sticky="nsew")
            ttk.Label(tile, text=label, font=(font_family, 11)).pack()
            value = ttk.Label(tile, text="--", font=(font_family, 26, "bold"))
            value.pack(pady=(4, 0))
            self._tiles[key] = value
        for i in range(4):
            self.grid_columnconfigure(i, weight=1)

    def update_values(self, cpu, gpu, t, speed):
        def fmt_temp(v):
            return f"{v:.0f} °C" if v is not None else "--"

        def fmt_speed(v):
            return f"{int(v)} %" if v is not None else "--"

        self._tiles["cpu"].configure(text=fmt_temp(cpu))
        self._tiles["gpu"].configure(text=fmt_temp(gpu))
        self._tiles["t"].configure(text=fmt_temp(t))
        self._tiles["speed"].configure(text=fmt_speed(speed))


class CurveEditor(ttk.Frame):
    """Matplotlib figure with draggable control points defining a fan curve."""

    HIT_RADIUS_TEMP = 3.0
    HIT_RADIUS_SPEED = 5.0

    def __init__(self, master, curve, on_change, font_prop=None, palette=None):
        super().__init__(master)
        self.on_change = on_change
        self.font_prop = font_prop
        self.palette = palette or LIGHT_PALETTE
        self.curve = [list(p) for p in curve]

        self.fig, self.ax = plt.subplots(figsize=(7, 5))
        self.canvas = FigureCanvasTkAgg(self.fig, master=self)
        self.canvas.get_tk_widget().pack(fill=tk.BOTH, expand=True)

        self.line, = self.ax.plot([], [], '-', linewidth=2)
        self.scatter = self.ax.scatter([], [], s=120, zorder=5, edgecolors="white", linewidths=1.5)

        self._dragging_idx = None
        self.canvas.mpl_connect('button_press_event', self._on_press)
        self.canvas.mpl_connect('motion_notify_event', self._on_motion)
        self.canvas.mpl_connect('button_release_event', self._on_release)

        self.apply_palette(self.palette)
        self._redraw()

    def apply_palette(self, palette):
        self.palette = palette
        self.fig.patch.set_facecolor(palette["bg"])
        self.ax.set_facecolor(palette["bg"])
        for spine in self.ax.spines.values():
            spine.set_color(palette["fg"])
        self.ax.tick_params(colors=palette["fg"])
        self.ax.xaxis.label.set_color(palette["fg"])
        self.ax.yaxis.label.set_color(palette["fg"])
        self.ax.title.set_color(palette["fg"])
        self.line.set_color(palette["curve"])
        self.scatter.set_color(palette["point"])
        self.ax.set_xlim(CURVE_TEMP_MIN, CURVE_TEMP_MAX)
        self.ax.set_ylim(CURVE_SPEED_MIN - 5, CURVE_SPEED_MAX + 5)
        self.ax.grid(True, alpha=0.3, color=palette["grid"])
        kwargs = {"fontproperties": self.font_prop} if self.font_prop else {}
        self.ax.set_xlabel("温度 °C", **kwargs)
        self.ax.set_ylabel("风扇速度 %", **kwargs)
        self.ax.set_title("风扇曲线（拖动控制点）", **kwargs)
        self.canvas.draw_idle()

    def set_curve(self, curve):
        self.curve = [list(p) for p in curve]
        self._redraw()

    def _redraw(self):
        ts = [p[0] for p in self.curve]
        ss = [p[1] for p in self.curve]
        self.line.set_data(ts, ss)
        self.scatter.set_offsets(list(zip(ts, ss)))
        self.canvas.draw_idle()

    def _on_press(self, event):
        if event.inaxes != self.ax or event.xdata is None:
            return
        for i, (t, s) in enumerate(self.curve):
            if (abs(event.xdata - t) <= self.HIT_RADIUS_TEMP
                    and abs(event.ydata - s) <= self.HIT_RADIUS_SPEED):
                self._dragging_idx = i
                return

    def _on_motion(self, event):
        if self._dragging_idx is None or event.inaxes != self.ax or event.xdata is None:
            return
        i = self._dragging_idx
        t = max(CURVE_TEMP_MIN, min(CURVE_TEMP_MAX, event.xdata))
        s = max(CURVE_SPEED_MIN, min(CURVE_SPEED_MAX, event.ydata))
        if i > 0:
            t = max(t, self.curve[i - 1][0] + 0.5)
        if i < len(self.curve) - 1:
            t = min(t, self.curve[i + 1][0] - 0.5)
        self.curve[i] = [round(t, 1), round(s, 1)]
        self._redraw()

    def _on_release(self, _event):
        if self._dragging_idx is not None:
            self._dragging_idx = None
            self.on_change([list(p) for p in self.curve])


# ---------- App ----------
class App:
    def __init__(self, root):
        self.root = root
        self.root.title("风扇控制器")
        self.root.geometry("1280x780")
        self.root.minsize(960, 600)

        self.config = load_config()
        self._tray_icon = None

        try:
            self.font_prop = fm.FontProperties(fname=FONT_PATH)
        except Exception:
            self.font_prop = None

        self._apply_theme(self.config.get("theme", "light"), redraw=False)

        self.controller = FanController(self.config["curve"], self.config["strategy"])
        if self.config.get("manual_enabled"):
            self.controller.set_manual(self.config.get("manual_speed", 50))

        self._build_layout()
        self._update_plot()
        self._update_status()

        self.root.protocol("WM_DELETE_WINDOW", self.on_closing)
        if self.config.get("minimize") == 1:
            self.minimize_to_tray()

    # ---- Layout ----
    def _build_layout(self):
        self.status_bar = StatusBar(self.root)
        self.status_bar.pack(side=tk.TOP, fill=tk.X, padx=15, pady=(15, 5))

        body = ttk.Frame(self.root)
        body.pack(side=tk.TOP, fill=tk.BOTH, expand=True, padx=15, pady=10)

        self._build_left_panel(body)
        self._build_right_panel(body)

    def _build_left_panel(self, parent):
        panel = ttk.Frame(parent, padding=10)
        panel.pack(side=tk.LEFT, fill=tk.Y)

        # Preset buttons
        ttk.Label(panel, text="预设方案").pack(anchor="w", pady=(0, 4))
        preset_frame = ttk.Frame(panel)
        preset_frame.pack(fill=tk.X, pady=(0, 14))
        for key, label, _curve, _strategy in PRESETS:
            ttk.Button(preset_frame, text=label,
                       command=lambda k=key: self._apply_preset(k)
                       ).pack(side=tk.LEFT, expand=True, fill=tk.X, padx=2)

        # Strategy
        ttk.Label(panel, text="温度策略").pack(anchor="w", pady=(0, 4))
        self.strategy_var = tk.StringVar(value=self.config["strategy"])
        self._strategy_labels = {label: key for key, label in STRATEGIES}
        self._strategy_keys = {key: label for key, label in STRATEGIES}
        self.strategy_combo = ttk.Combobox(
            panel, textvariable=self.strategy_var, state="readonly",
            values=[label for _, label in STRATEGIES], width=28,
        )
        self.strategy_combo.set(self._strategy_keys.get(self.config["strategy"], STRATEGIES[0][1]))
        self.strategy_combo.bind("<<ComboboxSelected>>", self._on_strategy_change)
        self.strategy_combo.pack(fill=tk.X, pady=(0, 14))

        # Manual mode
        self.manual_var = tk.BooleanVar(value=bool(self.config.get("manual_enabled", False)))
        manual_check = ttk.Checkbutton(
            panel, text="手动模式（锁定转速）",
            variable=self.manual_var, command=self._on_manual_toggle,
        )
        manual_check.pack(anchor="w", pady=(0, 4))

        self.manual_speed_var = tk.IntVar(value=int(self.config.get("manual_speed", 50)))
        self.manual_value_label = ttk.Label(panel, text=f"{self.manual_speed_var.get()} %")
        self.manual_value_label.pack(anchor="w")
        self.manual_slider = ttk.Scale(
            panel, from_=0, to=100, orient=tk.HORIZONTAL,
            variable=self.manual_speed_var, command=self._on_manual_slide,
        )
        self.manual_slider.pack(fill=tk.X, pady=(0, 14))
        self._sync_manual_controls()

        # Display range
        ttk.Label(panel, text="历史记录时间范围（分钟）").pack(anchor="w", pady=(0, 4))
        self.time_entry_var = tk.StringVar(value=str(self.config.get("time_entry", "5")))
        time_entry = ttk.Entry(panel, textvariable=self.time_entry_var)
        time_entry.pack(fill=tk.X, pady=(0, 14))

        # Buttons
        ttk.Button(panel, text="切换主题", command=self.toggle_theme).pack(fill=tk.X, pady=4)
        ttk.Button(panel, text="最小化到托盘", command=self.minimize_to_tray).pack(fill=tk.X, pady=4)
        self.startup_button = ttk.Button(
            panel,
            text="移除开机自启动" if is_in_startup(STARTUP_IDENTIFIER) else "添加开机自启动",
            command=self.toggle_startup,
        )
        self.startup_button.pack(fill=tk.X, pady=4)

    def _build_right_panel(self, parent):
        self.notebook = ttk.Notebook(parent)
        self.notebook.pack(side=tk.RIGHT, fill=tk.BOTH, expand=True, padx=(10, 0))

        # ----- Tab 1: history -----
        history_tab = ttk.Frame(self.notebook)
        self.notebook.add(history_tab, text="实时监控")
        self.fig, self.ax = plt.subplots(figsize=(8, 5))
        self.canvas = FigureCanvasTkAgg(self.fig, master=history_tab)
        self.canvas.get_tk_widget().pack(fill=tk.BOTH, expand=True)
        self._apply_chart_palette()

        # ----- Tab 2: curve editor -----
        curve_tab = ttk.Frame(self.notebook)
        self.notebook.add(curve_tab, text="风扇曲线")
        self.curve_editor = CurveEditor(
            curve_tab, self.config["curve"], on_change=self._on_curve_change,
            font_prop=self.font_prop, palette=self.palette,
        )
        self.curve_editor.pack(fill=tk.BOTH, expand=True)

    # ---- Theme ----
    def _apply_theme(self, theme, redraw=True):
        self.theme = theme if theme in ("light", "dark") else "light"
        if HAS_SV_TTK:
            sv_ttk.set_theme(self.theme)
        self.palette = palette_for(self.theme)
        if redraw:
            self._apply_chart_palette()
            if hasattr(self, "curve_editor"):
                self.curve_editor.apply_palette(self.palette)

    def toggle_theme(self):
        new_theme = "dark" if self.theme == "light" else "light"
        self._apply_theme(new_theme)
        self.config["theme"] = new_theme
        save_config(self.config)

    def _apply_chart_palette(self):
        if not hasattr(self, "ax"):
            return
        p = self.palette
        self.fig.patch.set_facecolor(p["bg"])
        self.ax.set_facecolor(p["bg"])
        for spine in self.ax.spines.values():
            spine.set_color(p["fg"])
        self.ax.tick_params(colors=p["fg"])
        self.canvas.draw_idle()

    # ---- Event handlers ----
    def _apply_preset(self, key):
        for k, _label, curve, strategy in PRESETS:
            if k != key:
                continue
            new_curve = [list(p) for p in curve]
            self.config["curve"] = new_curve
            self.config["strategy"] = strategy
            self.controller.set_curve(new_curve)
            self.controller.set_strategy(strategy)
            if hasattr(self, "curve_editor"):
                self.curve_editor.set_curve(new_curve)
            self.strategy_combo.set(self._strategy_keys.get(strategy, STRATEGIES[0][1]))
            save_config(self.config)
            return

    def _on_strategy_change(self, event):
        label = event.widget.get()
        key = self._strategy_labels.get(label, DEFAULT_STRATEGY)
        self.config["strategy"] = key
        self.controller.set_strategy(key)
        save_config(self.config)

    def _on_manual_toggle(self):
        enabled = self.manual_var.get()
        self.config["manual_enabled"] = enabled
        self._sync_manual_controls()
        if enabled:
            self.controller.set_manual(self.manual_speed_var.get())
        else:
            self.controller.set_manual(None)
        save_config(self.config)

    def _on_manual_slide(self, value):
        # ttk.Scale passes a string
        v = int(float(value))
        self.manual_speed_var.set(v)
        self.manual_value_label.configure(text=f"{v} %")
        self.config["manual_speed"] = v
        if self.manual_var.get():
            self.controller.set_manual(v)
        save_config(self.config)

    def _sync_manual_controls(self):
        state = "normal" if self.manual_var.get() else "disabled"
        self.manual_slider.configure(state=state)

    def _on_curve_change(self, curve):
        self.config["curve"] = curve
        self.controller.set_curve(curve)
        save_config(self.config)

    # ---- Periodic updates ----
    def _update_status(self):
        cpu, gpu, t, speed = self.controller.latest
        self.status_bar.update_values(cpu, gpu, t, speed)
        if self._tray_icon is not None:
            try:
                self._tray_icon.icon = self._tray_icon_image(t)
                self._tray_icon.title = self._tray_tooltip(t, speed)
            except Exception:
                pass
        self._status_after_id = self.root.after(STATUS_REFRESH_MS, self._update_status)

    def _update_plot(self):
        if self.root.state() == 'normal':
            self._plot_history()
        self._plot_after_id = self.root.after(PLOT_REFRESH_MS, self._update_plot)

    def _plot_history(self):
        history = self.controller.snapshot()
        if not history:
            return
        try:
            minutes = int(self.time_entry_var.get())
        except ValueError:
            minutes = None
        if minutes:
            cutoff = datetime.datetime.now() - datetime.timedelta(minutes=minutes)
            history = [entry for entry in history if entry[0] >= cutoff]
        if not history:
            return
        times, cpu_temps, gpu_temps, ts, speeds = zip(*history)
        p = self.palette
        self.ax.clear()
        self.ax.set_facecolor(p["bg"])
        self.ax.plot(times, cpu_temps, label='CPU 温度', color=p["cpu"])
        self.ax.plot(times, gpu_temps, label='GPU 温度', color=p["gpu"])
        self.ax.plot(times, ts, label='加权温度', color=p["t"])
        self.ax.plot(times, speeds, label='风扇速度', color=p["speed"])
        for spine in self.ax.spines.values():
            spine.set_color(p["fg"])
        self.ax.tick_params(colors=p["fg"])
        self.ax.grid(True, alpha=0.3, color=p["grid"])
        kwargs = {"fontproperties": self.font_prop} if self.font_prop else {}
        self.ax.set_xlabel('时间', color=p["fg"], **kwargs)
        self.ax.set_ylabel('数值', color=p["fg"], **kwargs)
        self.ax.set_title('温度和风扇速度历史记录', color=p["fg"], **kwargs)
        legend = self.ax.legend(prop=self.font_prop, facecolor=p["bg"],
                                edgecolor=p["grid"], labelcolor=p["fg"])
        if legend:
            for text in legend.get_texts():
                text.set_color(p["fg"])
        self.canvas.draw_idle()

    # ---- Lifecycle ----
    def _persist_config(self):
        self.config["time_entry"] = self.time_entry_var.get()
        save_config(self.config)

    def stop(self):
        if getattr(self, "_plot_after_id", None) is not None:
            self.root.after_cancel(self._plot_after_id)
        if getattr(self, "_status_after_id", None) is not None:
            self.root.after_cancel(self._status_after_id)
        self.controller.stop()

    def on_closing(self):
        self._persist_config()
        self.root.destroy()
        self.stop()

    @staticmethod
    def _tray_icon_image(temp=None, size=64):
        """Build a tray icon. If temp is given, draws a colored number; otherwise a placeholder."""
        img = Image.new('RGBA', (size, size), (28, 28, 28, 255))
        dc = ImageDraw.Draw(img)
        text = "--" if temp is None else str(int(round(temp)))
        if temp is None:
            color = (200, 200, 200)
        elif temp < 60:
            color = (80, 220, 100)
        elif temp < 80:
            color = (255, 200, 50)
        else:
            color = (255, 90, 90)
        # pick a font size that comfortably fits 2-3 chars
        font = None
        for size_px in (44, 40, 36):
            try:
                font = ImageFont.truetype("arial.ttf", size_px)
                bbox = dc.textbbox((0, 0), text, font=font)
                if bbox[2] - bbox[0] <= size - 4:
                    break
            except OSError:
                font = None
        if font is None:
            font = ImageFont.load_default()
        bbox = dc.textbbox((0, 0), text, font=font)
        w, h = bbox[2] - bbox[0], bbox[3] - bbox[1]
        x = (size - w) / 2 - bbox[0]
        y = (size - h) / 2 - bbox[1]
        dc.text((x, y), text, fill=color, font=font)
        return img

    def on_quit(self, icon):
        self._persist_config()
        icon.stop()
        self._tray_icon = None
        self.on_closing()

    def show_window(self, icon):
        self.root.deiconify()
        icon.stop()
        self._tray_icon = None
        self.config["minimize"] = 0
        save_config(self.config)

    def minimize_to_tray(self):
        if self._tray_icon is not None:
            return  # already in tray
        self.config["minimize"] = 1
        save_config(self.config)
        self.root.withdraw()
        cpu, gpu, t, speed = self.controller.latest
        menu = (pystray.MenuItem('显示主界面', self.show_window),
                pystray.MenuItem('退出', self.on_quit))
        self._tray_icon = pystray.Icon(
            "fan-controller", self._tray_icon_image(t),
            self._tray_tooltip(t, speed), menu,
        )
        self._tray_icon.run_detached()

    @staticmethod
    def _tray_tooltip(temp, speed):
        if temp is None:
            return "风扇控制器"
        return f"风扇控制器: {temp:.0f}°C / {int(speed) if speed is not None else '--'}%"

    def toggle_startup(self):
        target = os.path.join(APP_DIR, 'aaa.exe')
        if is_in_startup(STARTUP_IDENTIFIER):
            if remove_from_startup(STARTUP_IDENTIFIER):
                messagebox.showinfo("启动项", "启动项已成功移除")
                self.startup_button.configure(text="添加开机自启动")
            else:
                messagebox.showerror("启动项", "启动项移除失败，请尝试关闭杀毒软件或反馈错误信息")
        else:
            if add_to_startup(target, STARTUP_IDENTIFIER):
                messagebox.showinfo("启动项", "启动项已成功添加")
                self.startup_button.configure(text="移除开机自启动")
            else:
                messagebox.showerror("启动项", "启动项添加失败，请尝试关闭杀毒软件或反馈错误信息")


if __name__ == "__main__":
    root = tk.Tk()
    app = App(root)
    root.mainloop()
