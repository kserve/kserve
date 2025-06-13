# Copyright 2022 The KServe Authors.
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

import json
import logging.config
from typing import Optional, Union, Dict

import yaml


KSERVE_STORAGE_LOGLEVEL = "DEBUG"
KSERVE_STORAGE_LOGGER_NAME = "storage.initializer"
KSERVE_STORAGE_TRACE_LOGGER_NAME = "storage.initializer.trace"
KSERVE_STORAGE_LOGGER_FORMAT = (
    "%(asctime)s.%(msecs)03d %(process)s %(name)s "
    "%(levelname)s [%(filename)s:%(funcName)s():%(lineno)s] %(message)s"
)
KSERVE_STORAGE_TRACE_LOGGER_FORMAT = (
    "%(asctime)s.%(msecs)03d %(process)s %(name)s %(message)s"
)
KSERVE_STORAGE_LOGGER_DATE_FORMAT = "%Y-%m-%d %H:%M:%S"

KSERVE_STORAGE_LOG_CONFIG = {
    "version": 1,
    "disable_existing_loggers": False,
    "formatters": {
        "storage": {
            "()": "logging.Formatter",
            "fmt": KSERVE_STORAGE_LOGGER_FORMAT,
            "datefmt": KSERVE_STORAGE_LOGGER_DATE_FORMAT,
        },
        "storage_trace": {
            "()": "logging.Formatter",
            "fmt": KSERVE_STORAGE_TRACE_LOGGER_FORMAT,
            "datefmt": KSERVE_STORAGE_LOGGER_DATE_FORMAT,
        },
    },
    "handlers": {
        "storage": {
            "formatter": "storage",
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
        "storage_trace": {
            "formatter": "storage_trace",
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
    },
    "loggers": {
        "storage": {
            "handlers": ["storage"],
            "level": KSERVE_STORAGE_LOGLEVEL,
            "propagate": False,
        },
        "storage.trace": {
            "handlers": ["storage_trace"],
            "level": KSERVE_STORAGE_LOGLEVEL,
            "propagate": False,
        },
    },
}

logger = logging.getLogger(KSERVE_STORAGE_LOGGER_NAME)
trace_logger = logging.getLogger(KSERVE_STORAGE_TRACE_LOGGER_NAME)


def configure_logging(log_config: Optional[Union[Dict, str]] = None):
    """
    Configures Storage Initializer
    This function should be called before loading the model / starting the model
    server for consistent logging format.

    :param log_config: (Optional) File path or dict containing log config. If not provided default configuration
                       will be used. If explicitly set to None, the logger will not be configured.
                       - If a dictionary is provided, it will be used directly for configuring the logger.
                       - If a string is provided:
                           - If it ends with '.json', it will be treated as a path to a JSON file containing log
                             configuration.
                           - If it ends with '.yaml' or '.yml', it will be treated as a path to a YAML file containing
                             log configuration.
                           - Otherwise, it will be treated as a path to a configuration file in the format specified in
                             the Python logging module documentation. # See the note about fileConfig() here:
                             # https://docs.python.org/3/library/logging.config.html#configuration-file-format
    """
    if log_config is None:
        logging.config.dictConfig(KSERVE_STORAGE_LOG_CONFIG)
    elif isinstance(log_config, dict):
        logging.config.dictConfig(log_config)
    elif log_config.endswith(".json"):
        with open(log_config) as file:
            loaded_config = json.load(file)
            logging.config.dictConfig(loaded_config)
    elif log_config.endswith((".yaml", ".yml")):
        with open(log_config) as file:
            loaded_config = yaml.safe_load(file)
            logging.config.dictConfig(loaded_config)
    else:
        # See the note about fileConfig() here:
        # https://docs.python.org/3/library/logging.config.html#configuration-file-format
        logging.config.fileConfig(log_config, disable_existing_loggers=False)
