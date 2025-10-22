# Copyright 2025 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import functools
import logging
import time
from datetime import datetime

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


def log_execution(func):
    """Decorator to log function start/end with timestamps and duration."""

    @functools.wraps(func)
    def wrapper(*args, **kwargs):
        func_name = func.__name__

        timestamp_start = datetime.now().isoformat()
        logger.info(
            f"[{func_name}] [{timestamp_start}] start - args={args}, kwargs={kwargs}"
        )
        start_time = time.time()

        try:
            result = func(*args, **kwargs)
            duration = time.time() - start_time
            timestamp_end = datetime.now().isoformat()
            logger.info(f"[{func_name}] [{timestamp_end}] end - ✅ in {duration:.3f}s")
            return result
        except Exception as e:
            duration = time.time() - start_time
            timestamp_end = datetime.now().isoformat()
            logger.error(
                f"[{func_name}] [{timestamp_end}] end - ❌ {duration:.3f}s: {e}"
            )
            raise

    return wrapper
