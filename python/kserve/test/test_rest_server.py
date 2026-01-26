import pytest

from kserve import ModelServer, ModelRepository
from kserve.constants.constants import FASTAPI_APP_IMPORT_STRING
from kserve.protocol.rest.server import RESTServer


def get_server(event_loop: str) -> RESTServer:
    server = ModelServer(registered_models=ModelRepository())
    rest_server = RESTServer(
        FASTAPI_APP_IMPORT_STRING,
        server.dataplane,
        server.model_repository_extension,
        event_loop=event_loop,
        http_port=8080,
    )
    rest_server.create_application()
    return rest_server


@pytest.mark.parametrize("loop_value,expected", [
    ("auto", "auto"),
    ("asyncio", "asyncio"),
    ("uvloop", "uvloop"),
    ("invalid-value", "auto"),  # invalid falls back to 'auto'
])
def test_config_loop_value(loop_value, expected, monkeypatch):
    rs = get_server(event_loop=loop_value)
    assert rs.config.loop == expected
