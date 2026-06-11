import logging
import os


def validate_runtime_files(paths, show_error=False):
    missing = []
    for label, path in paths:
        if not os.path.exists(path):
            missing.append(f"{label}: {path}")
    for item in missing:
        logging.error("缺少必要运行文件: %s", item)
    if missing and show_error:
        import tkinter as tk
        from tkinter import messagebox
        root = tk.Tk()
        root.withdraw()
        messagebox.showerror("缺少运行文件", "缺少必要运行文件：\n" + "\n".join(missing))
        root.destroy()
    return not missing
