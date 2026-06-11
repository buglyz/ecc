import os
import sys

APP_NAME = 'FanController'
IS_FROZEN = getattr(sys, 'frozen', False)
APP_DIR = os.path.dirname(sys.executable if IS_FROZEN else os.path.abspath(__file__))
PROJECT_ROOT = os.path.abspath(os.path.join(APP_DIR, os.pardir, os.pardir))
ROOT_DIR = os.path.abspath(os.path.join(APP_DIR, os.pardir))
STATE_DIR = os.path.join(os.getenv('LOCALAPPDATA') or APP_DIR, APP_NAME)


def _first_existing_path(*paths):
    for path in paths:
        if os.path.exists(path):
            return path
    return paths[0]


EC_PROBE = _first_existing_path(
    os.path.join(APP_DIR, 'ec-probe.exe'),
    os.path.join(APP_DIR, '_internal', 'ec-probe.exe'),
    os.path.join(ROOT_DIR, 'ec-probe.exe'),
    os.path.join(PROJECT_ROOT, 'assets', 'ec-probe.exe'),
)
HWMON_DLL = _first_existing_path(
    os.path.join(APP_DIR, '_internal', 'data', 'LibreHardwareMonitorLib.dll'),
    os.path.join(APP_DIR, 'data', 'LibreHardwareMonitorLib.dll'),
    os.path.join(PROJECT_ROOT, 'assets', 'LibreHardwareMonitorLib.dll'),
)
CONFIG_PATH = os.path.join(STATE_DIR, 'config.json')
LEGACY_CONFIG_PATH = os.path.join(STATE_DIR, 'data.dat')
APP_LEGACY_CONFIG_PATHS = (
    os.path.join(APP_DIR, 'config.json'),
    os.path.join(APP_DIR, 'data.dat'),
)
LOG_PATH = os.path.join(STATE_DIR, 'fan_controller.log')
FONT_PATH = 'C:/Windows/Fonts/msyh.ttc'
STARTUP_TARGET = sys.executable if IS_FROZEN else os.path.abspath(sys.argv[0])
