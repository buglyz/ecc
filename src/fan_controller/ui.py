import datetime
import logging
import os
import tkinter as tk
from tkinter import messagebox, ttk

import matplotlib
matplotlib.use('Agg')
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

from .config import save_config, load_config
from .constants import (
    CURVE_TEMP_MAX, CURVE_TEMP_MIN, CURVE_SPEED_MAX, CURVE_SPEED_MIN,
    DEFAULT_STRATEGY, PRESETS, PLOT_REFRESH_MS, STATUS_REFRESH_MS,
    STARTUP_IDENTIFIER, STRATEGIES,
)
from .controller import FanController
from .paths import FONT_PATH, STARTUP_TARGET
from .startup import add_to_startup, is_in_startup, remove_from_startup

# ---------- Theme palettes for matplotlib ----------
DARK_PALETTE = {
    "bg": "#1c1c1c", "fg": "#e6e6e6", "grid": "#3a3a3a",
    "cpu": "#ff6b6b", "gpu": "#4dabf7", "t": "#ffd43b",
    "speed": "#51cf66", "curve": "#74c0fc", "point": "#74c0fc",
}
LIGHT_PALETTE = {
    "bg": "#f6f8fb", "fg": "#172033", "grid": "#d5dce8",
    "cpu": "#e03131", "gpu": "#1864ab", "t": "#f08c00",
    "speed": "#2b8a3e", "curve": "#2563eb", "point": "#2563eb",
}


def palette_for(theme):
    return DARK_PALETTE if theme == "dark" else LIGHT_PALETTE


# ---------- Widgets ----------
class StatusBar(ttk.Frame):
    def __init__(self, master, font_family="Segoe UI"):
        super().__init__(master, padding=(6, 6))
        self._tiles = {}
        labels = [
            ("cpu", "CPU", "处理器温度", "°C"),
            ("gpu", "GPU", "显卡温度", "°C"),
            ("t", "目标温度", "控制策略结果", "°C"),
            ("speed", "风扇", "当前输出", "%"),
        ]
        for i, (key, title, subtitle, unit) in enumerate(labels):
            tile = ttk.Frame(self, style="Card.TFrame", padding=(16, 12))
            tile.grid(row=0, column=i, padx=6, sticky="nsew")
            tile.grid_columnconfigure(0, weight=1)
            header = ttk.Frame(tile, style="Card.TFrame")
            header.grid(row=0, column=0, sticky="ew")
            ttk.Label(header, text=title, style="CardTitle.TLabel").pack(side=tk.LEFT)
            ttk.Label(header, text=unit, style="CardUnit.TLabel").pack(side=tk.RIGHT)
            value = ttk.Label(tile, text="--", style="Metric.TLabel", font=(font_family, 26, "bold"))
            value.grid(row=1, column=0, sticky="w", pady=(8, 1))
            ttk.Label(tile, text=subtitle, style="Hint.TLabel").grid(row=2, column=0, sticky="w")
            self._tiles[key] = value
            if key == "speed":
                self._fan_bar = ttk.Progressbar(
                    tile, length=130, maximum=100, mode="determinate"
                )
                self._fan_bar.grid(row=3, column=0, sticky="ew", pady=(8, 0))
        for i in range(4):
            self.grid_columnconfigure(i, weight=1, uniform="tile")

    def update_values(self, cpu, gpu, t, speed):
        def fmt_temp(v):
            return f"{v:.0f}" if v is not None else "--"

        def fmt_speed(v):
            return f"{int(v)}" if v is not None else "--"

        self._tiles["cpu"].configure(text=fmt_temp(cpu))
        self._tiles["gpu"].configure(text=fmt_temp(gpu))
        self._tiles["t"].configure(text=fmt_temp(t))
        self._tiles["speed"].configure(text=fmt_speed(speed))
        if speed is not None:
            self._fan_bar["value"] = max(0, min(100, int(speed)))
        else:
            self._fan_bar["value"] = 0


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

        self.fig, self.ax = plt.subplots(figsize=(8.2, 2.35))
        self.fig.subplots_adjust(left=0.06, right=0.97, top=0.86, bottom=0.22)
        self.canvas = FigureCanvasTkAgg(self.fig, master=self)
        self.canvas.get_tk_widget().pack(fill=tk.BOTH, expand=True)

        self.line, = self.ax.plot([], [], '-', linewidth=2.8)
        self.scatter = self.ax.scatter([], [], s=135, zorder=5, edgecolors="white", linewidths=1.6)

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
        for name, spine in self.ax.spines.items():
            if name in ("top", "right"):
                spine.set_visible(False)
            else:
                spine.set_visible(True)
                spine.set_color(palette["fg"])
                spine.set_linewidth(0.5)
                spine.set_alpha(0.7)
        self.ax.tick_params(colors=palette["fg"])
        self.ax.xaxis.label.set_color(palette["fg"])
        self.ax.yaxis.label.set_color(palette["fg"])
        self.ax.title.set_color(palette["fg"])
        self.line.set_color(palette["curve"])
        self.scatter.set_color(palette["point"])
        self.ax.set_xlim(CURVE_TEMP_MIN, CURVE_TEMP_MAX)
        self.ax.set_ylim(CURVE_SPEED_MIN - 5, CURVE_SPEED_MAX + 5)
        self.ax.grid(True, alpha=0.15, color=palette["grid"], linestyle='--')
        kwargs = {"fontproperties": self.font_prop} if self.font_prop else {}
        self.ax.set_xlabel("温度 °C", **kwargs)
        self.ax.set_ylabel("风扇 %", **kwargs)
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
        self.root.geometry("1420x900")
        self.root.minsize(1260, 780)

        self.config = load_config()
        self._tray_icon = None

        try:
            self.font_prop = fm.FontProperties(fname=FONT_PATH)
        except Exception:
            self.font_prop = None

        self._apply_theme(self.config.get("theme", "light"), redraw=False)
        self._configure_styles()

        self.controller = FanController(self.config["curve"], self.config["strategy"])
        if self.config.get("manual_enabled"):
            self.controller.set_manual(self.config.get("manual_speed", 50))

        self._build_layout()
        self._update_plot()
        self._update_status()

        self.root.protocol("WM_DELETE_WINDOW", self.on_closing)
        logging.info(
            "应用启动完成: theme=%s strategy=%s manual_enabled=%s manual_speed=%s",
            self.theme, self.config.get("strategy"),
            self.config.get("manual_enabled"), self.config.get("manual_speed"),
        )
        if self.config.get("minimize") == 1:
            self.minimize_to_tray()

    # ---- Layout ----
    def _configure_styles(self):
        style = ttk.Style(self.root)
        bg = self.palette["bg"]
        fg = self.palette["fg"]
        card_bg = "#ffffff" if self.theme == "light" else "#242424"
        muted = "#5b6473" if self.theme == "light" else "#a6a6a6"
        accent = self.palette["curve"]
        try:
            style.configure(".", font=("Segoe UI", 10))
            style.configure("TFrame", background=bg)
            style.configure("TLabel", background=bg, foreground=fg)
            style.configure("TLabelframe", background=bg, borderwidth=1, relief="solid")
            style.configure("TLabelframe.Label", background=bg, foreground=fg, font=("Segoe UI", 10, "bold"))
            style.configure("Card.TFrame", background=card_bg, relief="solid", borderwidth=1)
            style.configure("CardTitle.TLabel", background=card_bg, foreground=muted, font=("Segoe UI", 10, "bold"))
            style.configure("CardUnit.TLabel", background=card_bg, foreground=muted, font=("Segoe UI", 9))
            style.configure("Hint.TLabel", background=card_bg, foreground=muted, font=("Segoe UI", 9))
            style.configure("Metric.TLabel", background=card_bg, foreground=fg)
            style.configure("Hero.TLabel", background=bg, foreground=fg, font=("Segoe UI", 19, "bold"))
            style.configure("Subtle.TLabel", background=bg, foreground=muted, font=("Segoe UI", 9))
            style.configure("Accent.TButton", font=("Segoe UI", 10, "bold"))
            style.configure("TProgressbar", troughcolor=bg, background=accent)
        except tk.TclError:
            pass
        self.root.configure(bg=bg)

    def _build_layout(self):
        self.root.grid_rowconfigure(0, weight=0)
        self.root.grid_rowconfigure(1, weight=1)
        self.root.grid_rowconfigure(2, weight=0)
        self.root.grid_columnconfigure(0, weight=1)

        top = ttk.Frame(self.root)
        top.grid(row=0, column=0, sticky="ew", padx=20, pady=(14, 4))
        top.grid_columnconfigure(0, weight=1)
        top.grid_columnconfigure(1, weight=0)

        title_block = ttk.Frame(top)
        title_block.grid(row=0, column=0, sticky="w", pady=(0, 8))
        ttk.Label(title_block, text="风扇控制器", style="Hero.TLabel").pack(anchor="w")
        ttk.Label(title_block, text="实时温度监控 · 自动曲线调速 · 手动转速锁定", style="Subtle.TLabel").pack(anchor="w", pady=(2, 0))

        self._build_action_bar(top)
        self.status_bar = StatusBar(top)
        self.status_bar.grid(row=1, column=0, columnspan=2, sticky="ew")

        self.middle = ttk.Frame(self.root)
        self.middle.grid(row=1, column=0, sticky="nsew", padx=20, pady=6)
        self.middle.grid_rowconfigure(0, weight=1, minsize=300)
        self.middle.grid_columnconfigure(0, weight=1, minsize=720)
        self.middle.grid_columnconfigure(1, weight=0, minsize=330)

        self._build_chart_area(self.middle)
        self._build_controls_panel(self.middle)
        self._build_curve_section(self.root)

    def _build_action_bar(self, parent):
        action_bar = ttk.Frame(parent)
        action_bar.grid(row=0, column=1, sticky="ne", padx=(12, 0), pady=(2, 10))
        ttk.Button(action_bar, text="切换主题", command=self.toggle_theme, width=12
                   ).pack(side=tk.LEFT, padx=4)
        ttk.Button(action_bar, text="最小化到托盘", command=self.minimize_to_tray, width=14
                   ).pack(side=tk.LEFT, padx=4)
        self.startup_button = ttk.Button(
            action_bar,
            text="移除开机自启动" if is_in_startup(STARTUP_IDENTIFIER) else "添加开机自启动",
            command=self.toggle_startup,
            width=14,
        )
        self.startup_button.pack(side=tk.LEFT, padx=4)

    def _build_chart_area(self, parent):
        self.chart_box = ttk.LabelFrame(parent, text="实时监控", padding=12)
        self.chart_box.grid(row=0, column=0, sticky="nsew", padx=(0, 16))
        self.fig, self.ax = plt.subplots(figsize=(6.8, 3.5), constrained_layout=False)
        self.fig.subplots_adjust(left=0.08, right=0.97, top=0.86, bottom=0.18)
        self.canvas = FigureCanvasTkAgg(self.fig, master=self.chart_box)
        self.canvas.get_tk_widget().pack(fill=tk.BOTH, expand=True)
        self._apply_chart_palette()

    def _build_controls_panel(self, parent):
        self.controls_panel = ttk.Frame(parent)
        self.controls_panel.grid(row=0, column=1, sticky="new", padx=(0, 4))
        self.controls_panel.grid_columnconfigure(0, weight=1, minsize=310)

        # Preset
        self.preset_box = ttk.LabelFrame(self.controls_panel, text="预设方案", padding=(12, 10))
        self.preset_box.grid(row=0, column=0, sticky="ew", pady=(0, 10))
        self.preset_box.grid_columnconfigure((0, 1, 2), weight=1, uniform="preset")
        for index, (key, label, _curve, _strategy) in enumerate(PRESETS):
            ttk.Button(self.preset_box, text=label, style="Accent.TButton",
                       command=lambda k=key: self._apply_preset(k)
                       ).grid(row=0, column=index, sticky="ew", padx=3, ipady=3)

        # Strategy
        self.strategy_box = ttk.LabelFrame(self.controls_panel, text="温度策略", padding=(12, 10))
        self.strategy_box.grid(row=1, column=0, sticky="ew", pady=(0, 10))
        self.strategy_var = tk.StringVar(value=self.config["strategy"])
        self._strategy_labels = {label: key for key, label in STRATEGIES}
        self._strategy_keys = {key: label for key, label in STRATEGIES}
        self.strategy_combo = ttk.Combobox(
            self.strategy_box, textvariable=self.strategy_var, state="readonly",
            values=[label for _, label in STRATEGIES], width=24,
        )
        self.strategy_combo.set(self._strategy_keys.get(self.config["strategy"], STRATEGIES[0][1]))
        self.strategy_combo.bind("<<ComboboxSelected>>", self._on_strategy_change)
        self.strategy_combo.pack(fill=tk.X)

        # Manual mode
        self.manual_box = ttk.LabelFrame(self.controls_panel, text="手动模式", padding=(12, 10))
        self.manual_box.grid(row=2, column=0, sticky="ew", pady=(0, 10))
        self.manual_var = tk.BooleanVar(value=bool(self.config.get("manual_enabled", False)))
        ttk.Checkbutton(self.manual_box, text="锁定转速",
                        variable=self.manual_var, command=self._on_manual_toggle,
                        ).pack(anchor="w", pady=(0, 6))
        self.manual_speed_var = tk.IntVar(value=int(self.config.get("manual_speed", 50)))
        value_row = ttk.Frame(self.manual_box)
        value_row.pack(fill=tk.X, pady=(0, 6))
        ttk.Label(value_row, text="当前转速", style="Subtle.TLabel").pack(side=tk.LEFT)
        self.manual_value_label = ttk.Label(value_row, text=f"{self.manual_speed_var.get()} %", font=("Segoe UI", 12, "bold"))
        self.manual_value_label.pack(side=tk.RIGHT)
        self.manual_slider = ttk.Scale(
            self.manual_box, from_=0, to=100, orient=tk.HORIZONTAL,
            variable=self.manual_speed_var, command=self._on_manual_slide,
        )
        self.manual_slider.pack(fill=tk.X, pady=(0, 6))
        ttk.Label(self.manual_box, text="恢复自动曲线控制", style="Subtle.TLabel").pack(anchor="w")
        self._sync_manual_controls()

        # History range
        self.range_box = ttk.LabelFrame(self.controls_panel, text="历史范围（分钟）", padding=(12, 10))
        self.range_box.grid(row=3, column=0, sticky="ew", pady=(0, 10))
        self.time_entry_var = tk.StringVar(value=str(self.config.get("time_entry", "5")))
        ttk.Entry(self.range_box, textvariable=self.time_entry_var).pack(fill=tk.X)

    def _build_curve_section(self, parent):
        self.curve_box = ttk.LabelFrame(parent, text="风扇曲线（拖动控制点）", padding=12)
        self.curve_box.grid(row=2, column=0, sticky="ew", padx=20, pady=(2, 14))
        self.curve_editor = CurveEditor(
            self.curve_box, self.config["curve"], on_change=self._on_curve_change,
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
            self._configure_styles()
            if hasattr(self, "curve_editor"):
                self.curve_editor.apply_palette(self.palette)

    def toggle_theme(self):
        new_theme = "dark" if self.theme == "light" else "light"
        self._apply_theme(new_theme)
        self.config["theme"] = new_theme
        save_config(self.config)

    def _style_axes(self, title=None):
        p = self.palette
        self.ax.set_facecolor(p["bg"])
        for name, spine in self.ax.spines.items():
            if name in ("top", "right"):
                spine.set_visible(False)
            else:
                spine.set_visible(True)
                spine.set_color(p["fg"])
                spine.set_linewidth(0.5)
                spine.set_alpha(0.7)
        self.ax.tick_params(colors=p["fg"], labelsize=9)
        self.ax.grid(True, alpha=0.15, color=p["grid"], linestyle='--')
        kwargs = {"fontproperties": self.font_prop} if self.font_prop else {}
        self.ax.set_xlabel('时间', color=p["fg"], **kwargs)
        self.ax.set_ylabel('数值', color=p["fg"], **kwargs)
        if title:
            self.ax.set_title(title, color=p["fg"], fontsize=11, **kwargs)

    def _draw_empty_chart(self):
        p = self.palette
        self.ax.clear()
        self._style_axes('温度与风扇速度趋势')
        self.ax.set_xlim(0, 1)
        self.ax.set_ylim(0, 100)
        self.ax.text(
            0.5, 0.5, "等待温度数据...", transform=self.ax.transAxes,
            ha="center", va="center", color=p["fg"], alpha=0.65,
            fontsize=12,
            fontproperties=self.font_prop if self.font_prop else None,
        )
        self.canvas.draw_idle()

    def _apply_chart_palette(self):
        if not hasattr(self, "ax"):
            return
        self.fig.patch.set_facecolor(self.palette["bg"])
        self._draw_empty_chart()

    # ---- Event handlers ----
    def _save_active_preset_defaults(self):
        key = self.config.get("active_preset")
        if key not in self.config.get("presets", {}):
            return
        self.config["presets"][key] = {
            "curve": [list(p) for p in self.config["curve"]],
            "strategy": self.config["strategy"],
        }

    def _apply_preset(self, key):
        preset = self.config.get("presets", {}).get(key)
        if preset is None:
            return
        new_curve = [list(p) for p in preset["curve"]]
        strategy = preset.get("strategy", DEFAULT_STRATEGY)
        self.config["active_preset"] = key
        self.config["curve"] = new_curve
        self.config["strategy"] = strategy
        self.controller.set_curve(new_curve)
        self.controller.set_strategy(strategy)
        if hasattr(self, "curve_editor"):
            self.curve_editor.set_curve(new_curve)
        self.strategy_combo.set(self._strategy_keys.get(strategy, STRATEGIES[0][1]))
        logging.info("用户应用预设: key=%s strategy=%s curve=%s", key, strategy, new_curve)
        save_config(self.config)

    def _on_strategy_change(self, event):
        label = event.widget.get()
        key = self._strategy_labels.get(label, DEFAULT_STRATEGY)
        self.config["strategy"] = key
        self.controller.set_strategy(key)
        self._save_active_preset_defaults()
        logging.info("用户切换温度策略: label=%s key=%s", label, key)
        save_config(self.config)

    def _on_manual_toggle(self):
        enabled = self.manual_var.get()
        self.config["manual_enabled"] = enabled
        self._sync_manual_controls()
        if enabled:
            self.controller.set_manual(self.manual_speed_var.get())
        else:
            self.controller.set_manual(None)
        logging.info("用户切换手动模式: enabled=%s speed=%s", enabled, self.manual_speed_var.get())
        save_config(self.config)

    def _on_manual_slide(self, value):
        # ttk.Scale passes a string
        v = int(float(value))
        self.manual_speed_var.set(v)
        self.manual_value_label.configure(text=f"{v} %")
        self.config["manual_speed"] = v
        if self.manual_var.get():
            self.controller.set_manual(v)
            logging.info("用户调整手动转速: speed=%s", v)
        save_config(self.config)

    def _sync_manual_controls(self):
        state = "normal" if self.manual_var.get() else "disabled"
        self.manual_slider.configure(state=state)

    def _on_curve_change(self, curve):
        self.config["curve"] = curve
        self.controller.set_curve(curve)
        self._save_active_preset_defaults()
        logging.info("用户调整风扇曲线: curve=%s", curve)
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
            self._draw_empty_chart()
            return
        try:
            minutes = int(self.time_entry_var.get())
        except ValueError:
            minutes = None
        if minutes:
            cutoff = datetime.datetime.now() - datetime.timedelta(minutes=minutes)
            history = [entry for entry in history if entry[0] >= cutoff]
        if not history:
            self._draw_empty_chart()
            return
        times, cpu_temps, gpu_temps, ts, speeds = zip(*history)
        p = self.palette
        self.ax.clear()
        self.ax.plot(times, cpu_temps, label='CPU 温度', color=p["cpu"], linewidth=1.8)
        self.ax.plot(times, gpu_temps, label='GPU 温度', color=p["gpu"], linewidth=1.8)
        self.ax.plot(times, ts, label='目标温度', color=p["t"], linewidth=2.2)
        self.ax.plot(times, speeds, label='风扇速度', color=p["speed"], linewidth=2.0)
        self._style_axes('温度与风扇速度趋势')
        legend = self.ax.legend(
            loc="upper left", ncol=4, fontsize=8,
            prop=self.font_prop, facecolor=p["bg"], edgecolor=p["grid"], labelcolor=p["fg"],
        )
        if legend:
            for text in legend.get_texts():
                text.set_color(p["fg"])
        self.canvas.draw_idle()

    # ---- Lifecycle ----
    def _persist_config(self):
        self.config["time_entry"] = self.time_entry_var.get()
        save_config(self.config)

    def stop(self):
        logging.info("应用停止")
        if getattr(self, "_plot_after_id", None) is not None:
            self.root.after_cancel(self._plot_after_id)
            self._plot_after_id = None
        if getattr(self, "_status_after_id", None) is not None:
            self.root.after_cancel(self._status_after_id)
            self._status_after_id = None
        if self._tray_icon is not None:
            try:
                self._tray_icon.stop()
            except Exception:
                pass
            self._tray_icon = None
        self.controller.stop()

    def on_closing(self):
        self._persist_config()
        self.stop()
        self.root.destroy()

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
        # Absolute path so PIL still finds the font when running from a PyInstaller exe
        font = None
        for font_file in ("arial.ttf", "arialbd.ttf", "segoeui.ttf"):
            font_path = os.path.join("C:/Windows/Fonts", font_file)
            for size_px in (44, 40, 36, 30):
                try:
                    candidate = ImageFont.truetype(font_path, size_px)
                except OSError:
                    candidate = None
                    break
                bbox = dc.textbbox((0, 0), text, font=candidate)
                if bbox[2] - bbox[0] <= size - 4:
                    font = candidate
                    break
            if font is not None:
                break
        if font is None:
            font = ImageFont.load_default()
        bbox = dc.textbbox((0, 0), text, font=font)
        w, h = bbox[2] - bbox[0], bbox[3] - bbox[1]
        x = (size - w) / 2 - bbox[0]
        y = (size - h) / 2 - bbox[1]
        dc.text((x, y), text, fill=color, font=font)
        return img

    def on_quit(self, icon):
        # pystray menu callbacks run on its own thread; bounce Tk work back to the main thread
        try:
            icon.stop()
        except Exception:
            pass
        self.root.after(0, self._after_tray_quit)

    def _after_tray_quit(self):
        self._persist_config()
        self.on_closing()

    def show_window(self, icon):
        try:
            icon.stop()
        except Exception:
            pass
        self.root.after(0, self._after_tray_show)

    def _after_tray_show(self):
        self._tray_icon = None
        self.config["minimize"] = 0
        save_config(self.config)
        self.root.deiconify()
        self.root.lift()
        self.root.focus_force()

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
        target = STARTUP_TARGET
        if is_in_startup(STARTUP_IDENTIFIER):
            ok, error = remove_from_startup(STARTUP_IDENTIFIER)
            if ok:
                messagebox.showinfo("启动项", "启动项已成功移除")
                self.startup_button.configure(text="添加开机自启动")
            else:
                messagebox.showerror("启动项", f"启动项移除失败：{error}")
        else:
            ok, error = add_to_startup(target, STARTUP_IDENTIFIER)
            if ok:
                messagebox.showinfo("启动项", "启动项已成功添加")
                self.startup_button.configure(text="移除开机自启动")
            else:
                messagebox.showerror("启动项", f"启动项添加失败：{error}")


