# 受控验证 0xFB00/0x0300 三个模式（狂暴=0 / 游戏=1 / 办公=2）。
# 每个模式保持 3 秒，读取 EC 0x2C/0x2D 与 GMWMI RPM，最后恢复初始模式。
import ctypes
import json
import os
import struct
import subprocess
import time

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
GMWMI = os.path.join(ROOT, "assets", "GmwmiCli.exe")
ECPROBE = os.path.join(ROOT, "assets", "ec-probe.exe")
OUT = os.path.join(os.environ.get("TEMP", HERE), "mode_test.txt")


def req(a0, a1, a2=0, a3=0, a4=0):
    b = bytearray(32)
    struct.pack_into("<HHIII", b, 0, a0, a1, a2, a3, a4)
    return b.hex()


def gmwmi(tag, h):
    p = subprocess.run([GMWMI, "xfer", h], capture_output=True, text=True, timeout=10)
    out = (p.stdout or "").strip()
    print(f"{tag}: rc={p.returncode} out={out} err={(p.stderr or '').strip()}", flush=True)
    return out


def read_ec(reg):
    p = subprocess.run([ECPROBE, "read", reg], capture_output=True, text=True, timeout=10)
    return (p.stdout or "").strip()


def read_rpm():
    out = gmwmi("sensors", req(0xFA00, 0x0200))
    if len(out) >= 40:
        b = bytes.fromhex(out)
        return {
            "cpu_temp": struct.unpack_from("<I", b, 4)[0] & 0xFF,
            "gpu_temp": struct.unpack_from("<I", b, 8)[0] & 0xFF,
            "cpu_rpm": struct.unpack_from("<I", b, 12)[0] & 0xFFFF,
            "gpu_rpm": struct.unpack_from("<I", b, 16)[0] & 0xFFFF,
        }
    return {}


def snapshot(tag):
    snap = {
        "tag": tag,
        "ec_2c": read_ec("0x2C"),
        "ec_2d": read_ec("0x2D"),
        "rpm": read_rpm(),
    }
    print(f"SNAP {tag}: {snap}", flush=True)
    return snap


def reg_mode():
    p = subprocess.run(["reg", "query", r"HKLM\SOFTWARE\WOW6432Node\NL5\ControlCenter", "/v", "ModeBeforeDC"], capture_output=True, text=True)
    for token in p.stdout.split():
        if token.lower().startswith("0x"):
            try:
                return int(token, 16)
            except ValueError:
                pass
    return 2


def main():
    initial = reg_mode()
    result = {"elevated": bool(ctypes.windll.shell32.IsUserAnAdmin()), "initial_mode": initial, "steps": []}
    try:
        result["steps"].append(snapshot(f"initial(mode={initial})"))
        for mode, name in ((0, "狂暴"), (1, "游戏"), (2, "办公")):
            gmwmi(f"SetPowerMode({mode}) {name}", req(0xFB00, 0x0300, mode))
            time.sleep(3)
            result["steps"].append(snapshot(f"mode={mode} {name}"))
    finally:
        gmwmi(f"restore SetPowerMode({initial})", req(0xFB00, 0x0300, initial))
        time.sleep(2)
        result["steps"].append(snapshot("restored"))
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    print("RESULT_FILE=" + OUT)


if __name__ == "__main__":
    main()
