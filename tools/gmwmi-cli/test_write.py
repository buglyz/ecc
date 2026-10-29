# 受控 GMWMI 写入验证：30% 保持 2 秒，然后立即交还 BIOS。
# 仅在管理员权限下运行，用来确认该机型是否接受 RW_GMWMI 的写命令。
import json
import os
import struct
import subprocess
import time

HERE = os.path.dirname(os.path.abspath(__file__))
EXE = os.path.abspath(os.path.join(HERE, "..", "..", "assets", "GmwmiCli.exe"))
OUT = os.path.join(os.environ.get("TEMP", HERE), "gmwmi_write_test.txt")


def req(a0, a1, a2=0, a3=0, a4=0):
    b = bytearray(32)
    struct.pack_into("<HHIII", b, 0, a0, a1, a2, a3, a4)
    return b.hex()


def call(tag, hexreq):
    p = subprocess.run([EXE, "xfer", hexreq], capture_output=True, text=True, timeout=10)
    out = (p.stdout or "").strip()
    err = (p.stderr or "").strip()
    print(f"{tag}: rc={p.returncode} out={out} err={err}", flush=True)
    return out


def sensors(tag):
    out = call(tag, req(0xFA00, 0x0200))
    if len(out) >= 40:
        b = bytes.fromhex(out)
        cpu = struct.unpack_from("<I", b, 4)[0] & 0xFF
        gpu = struct.unpack_from("<I", b, 8)[0] & 0xFF
        crpm = struct.unpack_from("<I", b, 12)[0] & 0xFFFF
        grpm = struct.unpack_from("<I", b, 16)[0] & 0xFFFF
        print(f"  => CPU {cpu}C {crpm}RPM, GPU {gpu}C {grpm}RPM", flush=True)
        return {"cpu_temp": cpu, "gpu_temp": gpu, "cpu_rpm": crpm, "gpu_rpm": grpm}
    return {}


def main():
    result = {"elevated": bool(_is_admin())}
    try:
        result["before"] = sensors("before")
        call("mode=1", req(0xFB00, 0x0206, 1))
        call("fan=30/30/255", req(0xFB00, 0x0205, 30, 30, 255))
        time.sleep(2)
        result["after"] = sensors("after-2s")
    finally:
        call("release-mode=0", req(0xFB00, 0x0206, 0))
        call("release-fan=255", req(0xFB00, 0x0205, 255, 255, 255))
        time.sleep(2)
        result["released"] = sensors("after-release")
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    print("RESULT_FILE=" + OUT, flush=True)


def _is_admin():
    try:
        import ctypes
        return bool(ctypes.windll.shell32.IsUserAnAdmin())
    except Exception:
        return False


if __name__ == "__main__":
    main()
