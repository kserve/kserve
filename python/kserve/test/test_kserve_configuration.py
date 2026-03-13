import copy
import logging
import pytest

from unittest.mock import Mock, patch
from kserve.configuration import Configuration


def test_configuration_defaults():
    config = Configuration()

    assert config.host == "http://localhost"
    assert config.api_key == {}
    assert config.api_key_prefix == {}
    assert config.username is None
    assert config.password is None
    assert config.discard_unknown_keys is False
    assert config.verify_ssl is True
    assert config.client_side_validation is True
    assert config.debug is False


def test_api_key_with_prefix():
    config = Configuration(
        api_key={"apiKey": "secret"},
        api_key_prefix={"apiKey": "Bearer"},
    )

    token = config.get_api_key_with_prefix("apiKey")
    assert token == "Bearer secret"


def test_api_key_without_prefix():
    config = Configuration(api_key={"apiKey": "secret"})

    token = config.get_api_key_with_prefix("apiKey")
    assert token == "secret"


def test_api_key_missing_identifier():
    config = Configuration()
    assert config.get_api_key_with_prefix("missing") is None


def test_basic_auth_token():
    config = Configuration(username="user", password="pass")

    token = config.get_basic_auth_token()
    assert token.startswith("Basic ")


def test_debug_true_sets_debug_logging():
    config = Configuration()

    config.debug = True

    assert config.debug is True
    for _, logger in config.logger.items():
        assert logger.level == logging.DEBUG


def test_debug_false_sets_warning_logging():
    config = Configuration()
    config.debug = True

    config.debug = False

    assert config.debug is False
    for _, logger in config.logger.items():
        assert logger.level == logging.WARNING


def test_logger_file_set_adds_file_handler(tmp_path):
    log_file = tmp_path / "test.log"
    config = Configuration()

    config.logger_file = str(log_file)

    assert config.logger_file == str(log_file)
    assert config.logger_file_handler is not None


def test_deepcopy_configuration():
    config = Configuration()
    config.api_key["key"] = "value"
    config.debug = True

    copied = copy.deepcopy(config)

    assert copied is not config
    assert copied.api_key == config.api_key
    assert copied.debug == config.debug
    assert copied.logger is not config.logger  # shallow copy check


def test_set_default_and_get_default_copy():
    config = Configuration(host="http://example.com")
    Configuration.set_default(config)

    new_config = Configuration.get_default_copy()

    assert new_config is not config
    assert new_config.host == "http://example.com"


def test_get_default_copy_without_default():
    Configuration._default = None

    config = Configuration.get_default_copy()

    assert isinstance(config, Configuration)
    assert config.host == "http://localhost"


def test_to_debug_report_contains_expected_fields():
    config = Configuration()

    report = config.to_debug_report()

    assert "Python SDK Debug Report" in report
    assert "Python Version" in report
    assert "Version of the API: v0.1" in report


def test_get_host_settings():
    config = Configuration()

    hosts = config.get_host_settings()

    assert isinstance(hosts, list)
    assert hosts[0]["url"] == "/"


def test_get_host_from_settings_invalid_index():
    config = Configuration()

    with pytest.raises(ValueError, match="Invalid index"):
        config.get_host_from_settings(1)

###############
def test_logger_format_getter_after_setter():
    config = Configuration()

    new_format = '%(levelname)s:%(message)s'
    config.logger_format = new_format

    assert config.logger_format == new_format

def test_auth_settings_returns_empty_dict():
    config = Configuration()

    auth = config.auth_settings()

    assert auth == {}
    assert isinstance(auth, dict)


def test_get_api_key_with_prefix_calls_refresh_hook():
    config = Configuration()
    config.api_key = {"apiKey": "secret"}

    refresh_hook = Mock()
    config.refresh_api_key_hook = refresh_hook

    token = config.get_api_key_with_prefix("apiKey")

    refresh_hook.assert_called_once_with(config)
    assert token == "secret"


def test_get_host_from_settings_valid_index_no_variables():
    config = Configuration()

    with patch.object(
        config,
        "get_host_settings",
        return_value=[
            {
                "url": "/",
                "description": "test",
                "variables": {},
            }
        ],
    ):
        url = config.get_host_from_settings(0)

    assert url == "/"

def test_get_host_from_settings_with_variable_replacement():
    config = Configuration()

    with patch.object(
        config,
        "get_host_settings",
        return_value=[
            {
                "url": "/{version}",
                "variables": {
                    "version": {
                        "default_value": "v1",
                    }
                },
            }
        ],
    ):
        url = config.get_host_from_settings(0, variables={"version": "v2"})

    assert url == "/v2"

def test_get_host_from_settings_invalid_enum_value():
    config = Configuration()

    with patch.object(
        config,
        "get_host_settings",
        return_value=[
            {
                "url": "/{env}",
                "variables": {
                    "env": {
                        "default_value": "prod",
                        "enum_values": ["prod", "staging"],
                    }
                },
            }
        ],
    ):
        with pytest.raises(ValueError, match="invalid value"):
            config.get_host_from_settings(0, variables={"env": "dev"})



