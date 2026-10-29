# 对照验证：GMWMI 写风扇后，EC 寄存器 0x2C 是否变化。
# 仅用于诊断；结束时会交还 BIOS 控制。
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
OUT = os.path.join(os.environ.get("TEMP", HERE), "wmi_vs_ec.txt")


def req(a0, a1, a2=0, a3=0, a4=0):
    b = bytearray(32)
    struct.pack_into("<HHIII", b, 0, a0, a1, a2, a3, a4)
    return b.hex()


def gmwmi(tag, h):
    p = subprocess.run([GMWMI, "xfer", h], capture_output=True, text=True, timeout=10)
    out = (p.stdout or "").strip()
    print(f"{tag}: rc={p.returncode} out={out} err={(p.stderr or '').strip()}", flush=True)
    return out


def ec_read(reg):
    p = subprocess.run([ECPROBE, "read", reg], capture_output=True, text=True, timeout=10)
    out = (p.stdout or "").strip()
    err = (p.stderr or "").strip()
    print(f"ec-probe read {reg}: rc={p.returncode} out={out} err={err}", flush=True)
    return out


def main():
    result = {"elevated": bool(ctypes.windll.shell32.IsUserAnAdmin())}
    try:
        result["ec_2c_before"] = ec_read("0x2C")
        result["ec_2d_before"] = ec_read("0x2D")
        gmwmi("mode=1", req(0xFB00, 0x0206, 1))
        gmwmi("fan=30/30/255", req(0xFB00, 0x0205, 30, 30, 255))
        time.sleep(1)
        result["ec_2c_after_wmi"] = ec_read("0x2C")
        result["ec_2d_after_wmi"] = ec_read("0x2D")
    finally:
        gmwmi("release-mode=0", req(0xFB00, 0x0206, 0))
        gmwmi("release-fan=255", req(0xFB00, 0x0205, 255, 255, 255))
        time.sleep(1)
        result["ec_2c_after_release"] = ec_read("0x2C")
        result["ec_2d_after_release"] = ec_read("0x2D")
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    print("RESULT_FILE=" + OUT)


if __name__ == "__main__":
    main()
