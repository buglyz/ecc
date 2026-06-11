import logging
import os
import subprocess

from .paths import EC_PROBE

_startupinfo = subprocess.STARTUPINFO()
_startupinfo.dwFlags = subprocess.STARTF_USESHOWWINDOW
_startupinfo.wShowWindow = subprocess.SW_HIDE


def ec_write(register, value_hex):
    if not os.path.exists(EC_PROBE):
        logging.error("EC 写入跳过，缺少 ec-probe.exe: %s", EC_PROBE)
        return False
    try:
        result = subprocess.run(
            [EC_PROBE, 'write', '-v', register, value_hex],
            startupinfo=_startupinfo,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=5,
        )
        if result.returncode != 0:
            logging.error("EC 写入命令失败: register=%s value=%s returncode=%d", register, value_hex, result.returncode)
            return False
        logging.info("EC 写入完成: register=%s value=%s", register, value_hex)
        return True
    except subprocess.TimeoutExpired:
        logging.error("EC 写入超时: register=%s value=%s", register, value_hex)
        return False
    except OSError:
        logging.exception("EC 写入命令启动失败: register=%s value=%s", register, value_hex)
        return False
