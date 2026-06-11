import ctypes
import os
import sys

from .paths import IS_FROZEN


def is_admin():
    try:
        return ctypes.windll.shell32.IsUserAnAdmin()
    except Exception:
        return False


def set_dpi_awareness():
    try:
        ctypes.windll.shcore.SetProcessDpiAwareness(2)
    except Exception:
        try:
            ctypes.windll.shcore.SetProcessDpiAwareness(1)
        except Exception:
            pass


def relaunch_as_admin():
    if IS_FROZEN:
        executable = sys.executable
        params = None
    else:
        executable = sys.executable
        params = f'"{os.path.abspath(sys.argv[0])}"'
    try:
        result = ctypes.windll.shell32.ShellExecuteW(None, "runas", executable, params, None, 1)
        return result > 32
    except Exception:
        return False


def require_admin():
    if is_admin():
        return True
    relaunch_as_admin()
    return False
