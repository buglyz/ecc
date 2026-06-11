import logging
import os
from logging.handlers import RotatingFileHandler

from .paths import LOG_PATH, STATE_DIR


def setup_logging():
    logger = logging.getLogger()
    if logger.handlers:
        return
    try:
        os.makedirs(STATE_DIR, exist_ok=True)
        handler = RotatingFileHandler(
            LOG_PATH, maxBytes=512 * 1024, backupCount=3, encoding='utf-8'
        )
        handler.setFormatter(logging.Formatter(
            '%(asctime)s %(levelname)s [%(threadName)s] %(message)s'
        ))
        logger.setLevel(logging.INFO)
        logger.addHandler(handler)
        logging.info("日志初始化完成: %s", LOG_PATH)
    except OSError:
        pass
