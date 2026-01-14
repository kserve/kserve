import json
import yaml
import logging.config
from unittest.mock import patch, mock_open
import pytest

# Import your function and constant
from kserve.logging import configure_logging, KSERVE_LOG_CONFIG


def test_configure_logging_none():
    """Should load default KSERVE_LOG_CONFIG when config=None."""
    with patch("logging.config.dictConfig") as mock_dict:
        configure_logging(None)
        mock_dict.assert_called_once_with(KSERVE_LOG_CONFIG)


def test_configure_logging_dict():
    """Should directly use dictionary config."""
    custom_config = {"version": 1, "handlers": {}}

    with patch("logging.config.dictConfig") as mock_dict:
        configure_logging(custom_config)
        mock_dict.assert_called_once_with(custom_config)


def test_configure_logging_json():
    """Should open JSON file and apply dictConfig."""
    json_content = {"version": 1}

    with patch("builtins.open", mock_open(read_data=json.dumps(json_content))):
        with patch("json.load", return_value=json_content) as mock_json:
            with patch("logging.config.dictConfig") as mock_dict:
                configure_logging("config.json")

                mock_json.assert_called_once()
                mock_dict.assert_called_once_with(json_content)


def test_configure_logging_yaml():
    """Should open YAML file and apply dictConfig."""
    yaml_content = {"version": 1}

    with patch("builtins.open", mock_open(read_data="version: 1")):
        with patch("yaml.safe_load", return_value=yaml_content) as mock_yaml:
            with patch("logging.config.dictConfig") as mock_dict:
                configure_logging("config.yaml")

                mock_yaml.assert_called_once()
                mock_dict.assert_called_once_with(yaml_content)


def test_configure_logging_file_config():
    """Should use logging.config.fileConfig for non-json/yaml configs."""
    with patch("logging.config.fileConfig") as mock_file_config:
        configure_logging("logging.ini")
        mock_file_config.assert_called_once_with(
            "logging.ini", disable_existing_loggers=False
        )
