import os
import tempfile
import six

from unittest.mock import patch, MagicMock
from datetime import datetime, date
import mimetypes

import pytest

from kserve.api_client import ApiClient
from kserve.configuration import Configuration
import kserve.models as kserve_models
from kserve.exceptions import ApiValueError
from kserve import rest

def test_api_client_init_defaults():
    client = ApiClient()

    # configuration
    assert isinstance(client.configuration, Configuration)

    # pool threads default
    assert client.pool_threads == 1

    # rest client should be created
    assert client.rest_client is not None

    # headers
    assert isinstance(client.default_headers, dict)
    assert client.default_headers["User-Agent"] == "OpenAPI-Generator/0.1/python"

    # cookie
    assert client.cookie is None

    # client-side validation inherited from configuration
    assert client.client_side_validation == client.configuration.client_side_validation

def test_api_client_init_with_custom_header_and_cookie():
    config = Configuration()

    client = ApiClient(
        configuration=config,
        header_name="X-Test-Header",
        header_value="test-value",
        cookie="test-cookie",
        pool_threads=5,
    )

    assert client.configuration is config
    assert client.pool_threads == 5

    # custom header added
    assert client.default_headers["X-Test-Header"] == "test-value"

    # user-agent still present
    assert client.default_headers["User-Agent"] == "OpenAPI-Generator/0.1/python"

    # cookie set
    assert client.cookie == "test-cookie"

def test_api_client_enter_returns_self():
    client = ApiClient()

    with client as ctx:
        assert ctx is client

def test_api_client_close_without_pool():
    client = ApiClient()

    # pool not created yet
    assert client._pool is None

    # should not raise
    client.close()

    assert client._pool is None

def test_api_client_close_with_pool():
    client = ApiClient(pool_threads=2)

    # force pool creation
    pool = client.pool
    assert pool is not None

    client.close()

    # pool should be cleaned up
    assert client._pool is None

def test_api_client_exit_calls_close():
    client = ApiClient()

    with patch.object(client, "close") as mock_close:
        client.__exit__(None, None, None)

    mock_close.assert_called_once()

##############################################################
# Tests for user_agent property
##############################################################

def test_api_client_user_agent_default():
    client = ApiClient()

    assert client.user_agent == "OpenAPI-Generator/0.1/python"

def test_api_client_user_agent_setter():
    client = ApiClient()

    client.user_agent = "My-Custom-Agent/1.0"

    assert client.user_agent == "My-Custom-Agent/1.0"
    assert client.default_headers["User-Agent"] == "My-Custom-Agent/1.0"

##############################################################
# Tests for set_default_header method
##############################################################

def test_api_client_set_default_header_adds_header():
    client = ApiClient()

    client.set_default_header("X-Test", "123")

    assert client.default_headers["X-Test"] == "123"

def test_api_client_set_default_header_overwrites_value():
    client = ApiClient()

    client.set_default_header("X-Test", "123")
    client.set_default_header("X-Test", "456")

    assert client.default_headers["X-Test"] == "456"

def test_api_client_set_default_header_preserves_existing_headers():
    client = ApiClient()
    original_user_agent = client.user_agent

    client.set_default_header("X-New", "value")

    assert client.user_agent == original_user_agent
    assert client.default_headers["X-New"] == "value"

class FakeResponse:
    def __init__(self, data=b'{"key": "value"}'):
        self.data = data
        self.status = 200

    def getheader(self, name):
        if name.lower() == "content-type":
            return "application/json"
        return None

    def getheaders(self):
        return {"content-type": "application/json"}


def test__call_api_success(monkeypatch):
    client = ApiClient()

    client.configuration.host = "http://test-host"
    client.configuration.safe_chars_for_path_param = ""

    client.sanitize_for_serialization = MagicMock(side_effect=lambda x: x)
    client.parameters_to_tuples = MagicMock(
        side_effect=lambda x, _: list(x.items()) if isinstance(x, dict) else x
    )
    client.update_params_for_auth = MagicMock()
    client.deserialize = MagicMock(return_value={"key": "value"})

    fake_response = FakeResponse(b'{"key": "value"}')
    client.request = MagicMock(return_value=fake_response)

    result = client._ApiClient__call_api(
        resource_path="/models/{name}",
        method="GET",
        path_params={"name": "test-model"},
        header_params={"X-Test": "1"},
        response_type="object",
        _return_http_data_only=True,
    )

    assert result == {"key": "value"}



##############################################################
# Tests for sanitize_for_serialization function
##############################################################
def test_sanitize_for_serialization_primitives():
    client = ApiClient()
    assert client.sanitize_for_serialization("text") == "text"
    assert client.sanitize_for_serialization(123) == 123
    assert client.sanitize_for_serialization(True) is True
    is_non_result = client.sanitize_for_serialization(None)
    assert is_non_result is None

def test_sanitize_for_serialization_datetime():
    client = ApiClient()
    dt = datetime(2024, 1, 1, 12, 30, 45)

    result = client.sanitize_for_serialization(dt)

    assert result == "2024-01-01T12:30:45"

def test_sanitize_for_serialization_list_and_dict():
    client = ApiClient()

    data = {
        "a": 1,
        "b": ["x", 2]
    }

    result = client.sanitize_for_serialization(data)

    assert result == {"a": 1, "b": ["x", 2]}

def test_sanitize_for_serialization_tuple():
    client = ApiClient()

    data = (1, "a", True)

    result = client.sanitize_for_serialization(data)

    assert result == (1, "a", True)
    assert isinstance(result, tuple)

class DummyOpenAPIModel:
    openapi_types = {
        "id": int,
        "name": str,
        "skip": str,
    }

    attribute_map = {
        "id": "id",
        "name": "name",
        "skip": "skip",
    }

    def __init__(self):
        self.id = 1
        self.name = "test"
        self.skip = None  # should be excluded


def test_sanitize_for_serialization_openapi_model():
    client = ApiClient()
    obj = DummyOpenAPIModel()

    result = client.sanitize_for_serialization(obj)

    assert result == {
        "id": 1,
        "name": "test",
    }

###############################################################
# test for call_api method delegation
###############################################################
def test_call_api_delegates_to_private_call():
    client = ApiClient()

    with patch.object(client, "_ApiClient__call_api") as mock_call:
        client.call_api(
            "/test/{id}",
            "GET",
            path_params={"id": "123"},
            header_params={"X-Test": "yes"},
        )

        mock_call.assert_called_once()

def test_call_api_passes_path_params_to_private_call():
    client = ApiClient()

    with patch.object(client, "_ApiClient__call_api") as mock_call:
        client.call_api(
            "/items/{item_id}",
            "GET",
            path_params={"item_id": "42"},
        )

        args, kwargs = mock_call.call_args

        # resource path is NOT substituted here
        assert args[0] == "/items/{item_id}"

        # path_params are passed positionally
        assert args[2] == {"item_id": "42"}


def test_call_api_merges_headers():
    client = ApiClient()
    client.set_default_header("X-Default", "default")

    with patch.object(client, "_ApiClient__call_api") as mock_call:
        client.call_api(
            "/test",
            "GET",
            header_params={"X-Custom": "custom"},
        )

        args, kwargs = mock_call.call_args

        headers = args[4]

        # call_api itself does NOT merge headers
        # merging happens inside __call_api
        assert headers == {"X-Custom": "custom"}


def test__call_api_merges_default_and_request_headers():
    client = ApiClient()
    client.set_default_header("X-Default", "default")

    # Mock REST call
    with patch.object(client, "request") as mock_request:
        mock_response = MagicMock()
        mock_response.getheader.return_value = None
        mock_response.data = b"{}"
        mock_response.status = 200
        mock_response.getheaders.return_value = {}

        mock_request.return_value = mock_response

        client._ApiClient__call_api(
            resource_path="/test",
            method="GET",
            header_params={"X-Custom": "custom"},
            _return_http_data_only=True,
        )

        # Extract headers passed to request()
        _, kwargs = mock_request.call_args
        headers = kwargs["headers"]

        assert headers["X-Default"] == "default"
        assert headers["X-Custom"] == "custom"
    
#################################################################
# Test for Deserialize method
#################################################################

def test_deserialize_file_response():
    client = ApiClient()
    response = MagicMock()

    with patch.object(
        client, "_ApiClient__deserialize_file", return_value="/tmp/file.txt"
    ) as mock_file:
        result = client.deserialize(response, "file")

        mock_file.assert_called_once_with(response)
        assert result == "/tmp/file.txt"

def test_deserialize_json_response():
    client = ApiClient()

    response = MagicMock()
    response.data = '{"name": "test"}'

    with patch.object(
        client, "_ApiClient__deserialize", return_value={"name": "test"}
    ) as mock_deserialize:
        result = client.deserialize(response, dict)

        mock_deserialize.assert_called_once_with({"name": "test"}, dict)
        assert result == {"name": "test"}

def test_deserialize_non_json_response():
    client = ApiClient()

    response = MagicMock()
    response.data = "plain-text-response"

    with patch.object(
        client, "_ApiClient__deserialize", return_value="plain-text-response"
    ) as mock_deserialize:
        result = client.deserialize(response, str)

        mock_deserialize.assert_called_once_with("plain-text-response", str)
        assert result == "plain-text-response"


#################################################################
# Test for Deserialize method
#################################################################

def test__deserialize_none_data():
    client = ApiClient()

    result = client._ApiClient__deserialize(None, str)

    assert result is None

def test__deserialize_list_type():
    client = ApiClient()

    with patch.object(
        client, "_ApiClient__deserialize", side_effect=lambda x, y: x
    ) as mock_deserialize:
        result = client._ApiClient__deserialize([1, 2, 3], "list[int]")

        assert result == [1, 2, 3]

def test__deserialize_dict_type():
    client = ApiClient()

    with patch.object(
        client, "_ApiClient__deserialize", side_effect=lambda x, y: x
    ) as mock_deserialize:
        result = client._ApiClient__deserialize(
            {"a": 1, "b": 2},
            "dict(str, int)"
        )

        assert result == {"a": 1, "b": 2}

def test__deserialize_native_type_string():
    client = ApiClient()

    result = client._ApiClient__deserialize("123", "int")

    assert result == 123
    assert isinstance(result, int)

def test__deserialize_primitive_type():
    client = ApiClient()

    with patch.object(
        client, "_ApiClient__deserialize_primitive", return_value=10
    ) as mock_primitive:
        result = client._ApiClient__deserialize("10", int)

        mock_primitive.assert_called_once_with("10", int)
        assert result == 10

def test__deserialize_object_type():
    client = ApiClient()

    with patch.object(
        client, "_ApiClient__deserialize_object", return_value={"x": 1}
    ) as mock_obj:
        result = client._ApiClient__deserialize({"x": 1}, object)

        mock_obj.assert_called_once_with({"x": 1})
        assert result == {"x": 1}

def test__deserialize_date_type():
    client = ApiClient()

    with patch.object(
        client,
        "_ApiClient__deserialize_date",
        return_value=date(2024, 1, 1),
    ) as mock_date:
        result = client._ApiClient__deserialize("2024-01-01", date)

        assert result == date(2024, 1, 1)
        mock_date.assert_called_once_with("2024-01-01")


def test__deserialize_model_fallback():
    client = ApiClient()
    fake_model = MagicMock()

    with patch.object(
        client, "_ApiClient__deserialize_model", return_value=fake_model
    ) as mock_model:
        result = client._ApiClient__deserialize({"a": 1}, MagicMock)

        mock_model.assert_called_once_with({"a": 1}, MagicMock)
        assert result is fake_model

def test__deserialize_list_string_type():
    client = ApiClient()

    result = client._ApiClient__deserialize(
        ["1", "2"], "list[str]"
    )

    assert result == ["1", "2"]

def test__deserialize_dict_string_type():
    client = ApiClient()

    result = client._ApiClient__deserialize(
        {"a": "1", "b": "2"}, "dict(str, int)"
    )

    assert result == {"a": 1, "b": 2}

def test__deserialize_model_by_name():
    client = ApiClient()

    # pick a simple generated model
    model_class_name = next(iter(kserve_models.__dict__))

    model_cls = getattr(kserve_models, model_class_name)

    if not hasattr(model_cls, "openapi_types"):
        pytest.skip("Model not suitable for deserialization test")

    data = {}
    result = client._ApiClient__deserialize(data, model_class_name)

    assert isinstance(result, model_cls)

def test__deserialize_datetime_type():
    client = ApiClient()

    fake_dt = datetime(2024, 1, 1, 10, 0, 0)

    with patch.object(
        client,
        "_ApiClient__deserialize_datetime",
        return_value=fake_dt,
    ) as mock_dt:
        result = client._ApiClient__deserialize(
            "2024-01-01T10:00:00",
            datetime,
        )

        assert result == fake_dt
        mock_dt.assert_called_once_with("2024-01-01T10:00:00")


##############################################################
# Tests for parameters_to_tuples method
##############################################################

def test_parameters_to_tuples_no_collection_format():
    client = ApiClient()

    params = {"a": 1, "b": 2}

    result = client.parameters_to_tuples(params, None)

    assert result == [("a", 1), ("b", 2)]

def test_parameters_to_tuples_multi_format():
    client = ApiClient()

    params = {"ids": [1, 2, 3]}
    collection_formats = {"ids": "multi"}

    result = client.parameters_to_tuples(params, collection_formats)

    assert result == [("ids", 1), ("ids", 2), ("ids", 3)]

def test_parameters_to_tuples_csv_format():
    client = ApiClient()

    params = {"ids": [1, 2, 3]}
    collection_formats = {"ids": "csv"}

    result = client.parameters_to_tuples(params, collection_formats)

    assert result == [("ids", "1,2,3")]

def test_parameters_to_tuples_ssv_format():
    client = ApiClient()

    params = {"ids": [1, 2, 3]}
    collection_formats = {"ids": "ssv"}

    result = client.parameters_to_tuples(params, collection_formats)

    assert result == [("ids", "1 2 3")]

def test_parameters_to_tuples_tsv_format():
    client = ApiClient()

    params = {"ids": [1, 2, 3]}
    collection_formats = {"ids": "tsv"}

    result = client.parameters_to_tuples(params, collection_formats)

    assert result == [("ids", "1\t2\t3")]

def test_parameters_to_tuples_pipes_format():
    client = ApiClient()

    params = {"ids": [1, 2, 3]}
    collection_formats = {"ids": "pipes"}

    result = client.parameters_to_tuples(params, collection_formats)

    assert result == [("ids", "1|2|3")]

def test_parameters_to_tuples_list_input():
    client = ApiClient()

    params = [("a", 1), ("b", 2)]

    result = client.parameters_to_tuples(params, None)

    assert result == [("a", 1), ("b", 2)]

########################################################
# Tests for files_parameters method
########################################################

def test_files_parameters_with_single_file(tmp_path):
    client = ApiClient()

    # Create a temporary file
    file_path = tmp_path / "test.txt"
    content = b"hello world"
    file_path.write_bytes(content)

    files = {
        "file": str(file_path)
    }

    result = client.files_parameters(files)

    assert len(result) == 1

    key, file_tuple = result[0]

    filename, filedata, mimetype = file_tuple

    assert key == "file"
    assert filename == "test.txt"
    assert filedata == content
    assert mimetype == (mimetypes.guess_type("test.txt")[0] or "application/octet-stream")

def test_files_parameters_with_multiple_files(tmp_path):
    client = ApiClient()

    file1 = tmp_path / "a.txt"
    file2 = tmp_path / "b.txt"

    file1.write_bytes(b"a")
    file2.write_bytes(b"b")

    files = {
        "files": [str(file1), str(file2)]
    }

    result = client.files_parameters(files)

    assert len(result) == 2
    assert result[0][0] == "files"
    assert result[1][0] == "files"

def test_files_parameters_skips_empty_values():
    client = ApiClient()

    files = {
        "file": None
    }

    result = client.files_parameters(files)

    assert result == []

def test_files_parameters_none():
    client = ApiClient()

    result = client.files_parameters(None)

    assert result == []

########
def test_call_api_returns_sync_result():
    client = ApiClient()

    with patch.object(
        client, "_ApiClient__call_api", return_value="sync-result"
    ) as mock_call:
        result = client.call_api(
            "/test",
            "GET",
        )

        mock_call.assert_called_once()
        assert result == "sync-result"

def test_call_api_returns_async_result():
    client = ApiClient()

    fake_async_result = object()

    with patch.object(client.pool, "apply_async", return_value=fake_async_result) as mock_async:
        result = client.call_api(
            "/test",
            "GET",
            async_req=True,
        )

        mock_async.assert_called_once()
        assert result is fake_async_result

###############################################################
# Tests for select_header_accept method
###############################################################
def test_select_header_accept_none():
    client = ApiClient()

    assert client.select_header_accept(None) is None
    assert client.select_header_accept([]) is None

def test_select_header_accept_prefers_json():
    client = ApiClient()

    result = client.select_header_accept([
        "text/plain",
        "Application/JSON"
    ])

    assert result == "application/json"

def test_select_header_accept_joins_headers():
    client = ApiClient()

    result = client.select_header_accept([
        "text/plain",
        "application/xml"
    ])

    assert result == "text/plain, application/xml"

################################################################
# Tests for select_header_content_type method
################################################################
def test_select_header_content_type_default():
    client = ApiClient()

    assert client.select_header_content_type(None) == "application/json"
    assert client.select_header_content_type([]) == "application/json"

def test_select_header_content_type_prefers_json():
    client = ApiClient()

    result = client.select_header_content_type([
        "text/plain",
        "Application/JSON"
    ])

    assert result == "application/json"

def test_select_header_content_type_wildcard():
    client = ApiClient()

    result = client.select_header_content_type([
        "*/*"
    ])

    assert result == "application/json"

def test_select_header_content_type_fallback_first():
    client = ApiClient()

    result = client.select_header_content_type([
        "text/plain",
        "application/xml"
    ])

    assert result == "text/plain"

###############################################################
# Tests for update_params_for_auth method
###############################################################
def mock_auth_settings():
    return {
        "apiKeyHeader": {
            "in": "header",
            "key": "X-API-Key",
            "value": "secret",
        },
        "apiKeyCookie": {
            "in": "cookie",
            "value": "session=abc123",
        },
        "apiKeyQuery": {
            "in": "query",
            "key": "api_key",
            "value": "secret",
        },
        "invalidAuth": {
            "in": "body",
            "key": "bad",
            "value": "oops",
        },
    }

def test_update_params_for_auth_no_auth():
    client = ApiClient()

    headers = {}
    querys = []

    client.update_params_for_auth(headers, querys, None)

    assert headers == {}
    assert querys == []

def test_update_params_for_auth_header(monkeypatch):
    client = ApiClient()

    monkeypatch.setattr(
        client.configuration,
        "auth_settings",
        lambda: mock_auth_settings(),
    )

    headers = {}
    querys = []

    client.update_params_for_auth(headers, querys, ["apiKeyHeader"])

    assert headers["X-API-Key"] == "secret"

def test_update_params_for_auth_cookie(monkeypatch):
    client = ApiClient()

    monkeypatch.setattr(
        client.configuration,
        "auth_settings",
        lambda: mock_auth_settings(),
    )

    headers = {}
    querys = []

    client.update_params_for_auth(headers, querys, ["apiKeyCookie"])

    assert headers["Cookie"] == "session=abc123"

def test_update_params_for_auth_query(monkeypatch):
    client = ApiClient()

    monkeypatch.setattr(
        client.configuration,
        "auth_settings",
        lambda: mock_auth_settings(),
    )

    headers = {}
    querys = []

    client.update_params_for_auth(headers, querys, ["apiKeyQuery"])

    assert ("api_key", "secret") in querys

def test_update_params_for_auth_invalid_location(monkeypatch):
    client = ApiClient()

    monkeypatch.setattr(
        client.configuration,
        "auth_settings",
        lambda: mock_auth_settings(),
    )

    with pytest.raises(ApiValueError):
        client.update_params_for_auth({}, [], ["invalidAuth"])


###############################################################
# test for deserialize file helper
###############################################################


class FakeResponse:
    def __init__(self, data, headers=None):
        self.data = data
        self.headers = headers or {}

    def getheader(self, name):
        return self.headers.get(name)


def test__deserialize_file_without_content_disposition(tmp_path):
    client = ApiClient()
    client.configuration.temp_folder_path = str(tmp_path)

    response = FakeResponse(b"file-content")

    path = client._ApiClient__deserialize_file(response)

    assert os.path.exists(path)
    with open(path, "rb") as f:
        assert f.read() == b"file-content"

def test__deserialize_file_with_content_disposition(tmp_path):
    client = ApiClient()
    client.configuration.temp_folder_path = str(tmp_path)

    response = FakeResponse(
        b"file-content",
        headers={
            "Content-Disposition": 'attachment; filename="test.txt"'
        }
    )

    path = client._ApiClient__deserialize_file(response)

    assert path.endswith("test.txt")
    assert os.path.exists(path)

    with open(path, "rb") as f:
        assert f.read() == b"file-content"

################################################################
# test for deserialize premitive helpers
################################################################

def test__deserialize_primitive_success():
    client = ApiClient()

    result = client._ApiClient__deserialize_primitive("5", int)

    assert result == 5

def test__deserialize_primitive_unicode_error():
    client = ApiClient()

    class BadType:
        def __call__(self, value):
            raise UnicodeEncodeError("utf-8", b"", 0, 1, "error")

    result = client._ApiClient__deserialize_primitive("test", BadType())

    assert result == six.text_type("test")

def test__deserialize_primitive_type_error():
    client = ApiClient()

    # int(None) raises TypeError
    result = client._ApiClient__deserialize_primitive(None, int)

    assert result is None

def test__deserialize_object_returns_same_value():
    client = ApiClient()

    obj = {"key": "value"}

    result = client._ApiClient__deserialize_object(obj)

    assert result is obj

################################################################
# Test for deserialize date/time helpers
################################################################

def test__deserialize_date_invalid_string_raises_exception():
    client = ApiClient()

    with pytest.raises(rest.ApiException) as exc:
        client._ApiClient__deserialize_date("not-a-date")

    assert exc.value.status == 0
    assert "Failed to parse" in exc.value.reason

def test__deserialize_date_import_error_returns_string():
    client = ApiClient()

    with patch("kserve.api_client.parse", side_effect=ImportError):
        result = client._ApiClient__deserialize_date("2024-01-01")

    assert result == "2024-01-01"


def test__deserialize_date_valid_string():
    client = ApiClient()

    result = client._ApiClient__deserialize_date("2024-01-01")

    assert result == date(2024, 1, 1)

################################################################
# Test for deserialize model
################################################################

class EmptyModel:
    openapi_types = {}
    attribute_map = {}

def test__deserialize_model_returns_data_when_no_openapi_types():
    client = ApiClient()
    data = {"a": 1}

    result = client._ApiClient__deserialize_model(data, EmptyModel)

    assert result == data

class SimpleModel:
    openapi_types = {
        "name": str,
        "age": int,
    }
    attribute_map = {
        "name": "name",
        "age": "age",
    }

    def __init__(self, name=None, age=None):
        self.name = name
        self.age = age

def test__deserialize_model_populates_fields():
    client = ApiClient()
    data = {"name": "Alice", "age": 30}

    result = client._ApiClient__deserialize_model(data, SimpleModel)

    assert isinstance(result, SimpleModel)
    assert result.name == "Alice"
    assert result.age == 30

def test__deserialize_model_ignores_missing_fields():
    client = ApiClient()
    data = {"name": "Bob"}  # age missing

    result = client._ApiClient__deserialize_model(data, SimpleModel)

    assert result.name == "Bob"
    assert result.age is None

class BaseModel:
    openapi_types = {}
    attribute_map = {}
    discriminator_value_class_map = {"child": "ChildModel"}

    def __init__(self):
        pass

    def get_real_child_model(self, data):
        return "ChildModel"

class ChildModel:
    openapi_types = {}
    attribute_map = {}

    def __init__(self):
        pass

def test__deserialize_model_with_discriminator():
    client = ApiClient()
    data = {"type": "child"}

    with patch.object(
        client,
        "_ApiClient__deserialize",
        return_value="child-instance",
    ) as mock_deserialize:
        result = client._ApiClient__deserialize_model(data, BaseModel)

        mock_deserialize.assert_called_once_with(data, "ChildModel")
        assert result == "child-instance"

###############################################################
# Test for Request method
###############################################################
@pytest.fixture
def client():
    client = ApiClient()
    client.rest_client = MagicMock()
    return client

def test_request_get(client):
    client.rest_client.GET.return_value = "get-response"

    result = client.request(
        "GET",
        "http://example.com",
        query_params=[("a", 1)],
        headers={"h": "v"},
    )

    client.rest_client.GET.assert_called_once_with(
        "http://example.com",
        query_params=[("a", 1)],
        _preload_content=True,
        _request_timeout=None,
        headers={"h": "v"},
    )
    assert result == "get-response"

def test_request_head(client):
    client.rest_client.HEAD.return_value = "head-response"

    result = client.request("HEAD", "http://example.com")

    client.rest_client.HEAD.assert_called_once()
    assert result == "head-response"

def test_request_options(client):
    client.rest_client.OPTIONS.return_value = "options-response"

    result = client.request("OPTIONS", "http://example.com")

    client.rest_client.OPTIONS.assert_called_once()
    assert result == "options-response"

def test_request_post(client):
    client.rest_client.POST.return_value = "post-response"

    result = client.request(
        "POST",
        "http://example.com",
        post_params=[("k", "v")],
        body={"x": 1},
    )

    client.rest_client.POST.assert_called_once()
    assert result == "post-response"

def test_request_put(client):
    client.rest_client.PUT.return_value = "put-response"

    result = client.request("PUT", "http://example.com")

    client.rest_client.PUT.assert_called_once()
    assert result == "put-response"

def test_request_patch(client):
    client.rest_client.PATCH.return_value = "patch-response"

    result = client.request("PATCH", "http://example.com")

    client.rest_client.PATCH.assert_called_once()
    assert result == "patch-response"

def test_request_delete(client):
    client.rest_client.DELETE.return_value = "delete-response"

    result = client.request("DELETE", "http://example.com")

    client.rest_client.DELETE.assert_called_once()
    assert result == "delete-response"

def test_request_invalid_method_raises_error(client):
    with pytest.raises(ApiValueError):
        client.request("TRACE", "http://example.com")
