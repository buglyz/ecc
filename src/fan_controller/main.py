import sys
import tkinter as tk

from .admin import require_admin, set_dpi_awareness
from .controller import load_hardware_monitor
from .logging_setup import setup_logging
from .paths import EC_PROBE, HWMON_DLL
from .runtime import validate_runtime_files
from .ui import App


def main():
    if not require_admin():
        sys.exit()
    set_dpi_awareness()
    setup_logging()
    if not validate_runtime_files(
        (("ec-probe.exe", EC_PROBE), ("LibreHardwareMonitorLib.dll", HWMON_DLL)),
        show_error=True,
    ):
        sys.exit(1)
    load_hardware_monitor()
    root = tk.Tk()
    app = App(root)
    root.mainloop()


if __name__ == "__main__":
    main()
