import datetime
import logging
import time
from collections import deque
from threading import Event, Lock, Thread

from .constants import (
    CPU_WEIGHT,
    DEFAULT_CURVE,
    EC_FAN_RELEASE,
    EC_REG_FAN1,
    EC_REG_FAN2,
    EXPECTED_CYCLE_DURATION,
    HEARTBEAT_SECONDS,
    HISTORY_MAX_SAMPLES,
    HYSTERESIS_TEMP,
    LOOP_DRIFT_TOLERANCE,
    SAMPLES_PER_CYCLE,
    SAMPLE_INTERVAL,
)
from .ec import ec_write
from .paths import HWMON_DLL

Hardware = None


def load_hardware_monitor():
    global Hardware
    if Hardware is None:
        import clr
        clr.AddReference(HWMON_DLL)
        from LibreHardwareMonitor import Hardware as HardwareModule
        Hardware = HardwareModule


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
    if cpu is None:
        return gpu
    if gpu is None:
        return cpu
    return (cpu - gpu) * CPU_WEIGHT + gpu


class FanController:
    def __init__(self, curve, strategy):
        load_hardware_monitor()
        self._curve_lock = Lock()
        self._curve = list(curve)
        self._strategy = strategy
        self._manual_speed = None

        self._computer = Hardware.Computer()
        self._computer.IsCpuEnabled = True
        self._computer.IsGpuEnabled = True
        self._computer.Open()
        self._hardware = list(self._computer.Hardware)

        self.history = deque(maxlen=HISTORY_MAX_SAMPLES)
        self._history_lock = Lock()
        self.latest = (None, None, None, None)

        self._stop_event = Event()
        self._thread = Thread(target=self._run, daemon=True)
        self._thread.start()

    def set_curve(self, curve):
        with self._curve_lock:
            self._curve = [list(p) for p in curve]
        logging.info("控制器曲线已更新: curve=%s", curve)

    def set_strategy(self, strategy):
        with self._curve_lock:
            self._strategy = strategy
        logging.info("控制器策略已更新: strategy=%s", strategy)

    def set_manual(self, speed):
        with self._curve_lock:
            self._manual_speed = None if speed is None else int(speed)
        logging.info("控制器手动模式已更新: speed=%s", speed)

    def snapshot(self):
        with self._history_lock:
            return list(self.history)

    def stop(self):
        self._stop_event.set()
        logging.info("停止控制器，释放 EC 风扇控制")
        ec_write(EC_REG_FAN1, EC_FAN_RELEASE)
        ec_write(EC_REG_FAN2, EC_FAN_RELEASE)

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

        c = None
        g = None
        if cpu_temps:
            c = cpu_temps[-2] if len(cpu_temps) >= 2 else cpu_temps[0]
        if gpu_temps:
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
        ec_write(EC_REG_FAN1, hex(current_speed))
        ec_write(EC_REG_FAN2, hex(current_speed))
        logging.info("控制器初始转速写入: speed=%s hex=%s", current_speed, hex(current_speed))

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
                    self.history.append((datetime.datetime.now(), c, g, t, current_speed))
                time.sleep(SAMPLE_INTERVAL)

            drifted = abs(time.time() - cycle_start - EXPECTED_CYCLE_DURATION) > LOOP_DRIFT_TOLERANCE
            heartbeat_due = (time.time() - last_write_ts) >= HEARTBEAT_SECONDS
            manual, strategy = self._current_mode()

            if manual is not None:
                target = manual
                avg_t = None
                mode = "manual"
                last_committed_temp = None
            elif cycle_temps:
                avg_t = sum(cycle_temps) / len(cycle_temps)
                temp_settled = (last_committed_temp is not None
                                and abs(avg_t - last_committed_temp) < HYSTERESIS_TEMP)
                if temp_settled and not (drifted or heartbeat_due):
                    cycle_start = time.time()
                    continue
                target = self._target_speed(avg_t)
                mode = f"auto:{strategy}"
                last_committed_temp = avg_t
            else:
                cycle_start = time.time()
                continue

            target = max(0, min(100, int(round(target))))
            if target != current_speed or drifted or heartbeat_due:
                current_speed = target
                speed_hex = hex(current_speed)
                ec_write(EC_REG_FAN1, speed_hex)
                ec_write(EC_REG_FAN2, speed_hex)
                logging.info(
                    "转速已提交: mode=%s speed=%s hex=%s avg_temp=%s drifted=%s heartbeat=%s",
                    mode, current_speed, speed_hex,
                    f"{avg_t:.1f}" if avg_t is not None else None,
                    drifted, heartbeat_due,
                )
                last_write_ts = time.time()
            cycle_start = time.time()
