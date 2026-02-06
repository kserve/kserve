# Copyright 2023 The KServe Authors.
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

from unittest.mock import Mock

import pytest

from kserve.protocol.rest import server as rest_mod


@pytest.mark.parametrize(
    "loop_value,expected",
    [
        ("auto", "auto"),
        ("asyncio", "asyncio"),
        ("uvloop", "uvloop"),
        ("invalid-value", "auto"),  # invalid falls back to 'auto'
    ],
)
def test_config_loop_value(loop_value, expected, monkeypatch):
    monkeypatch.setattr(rest_mod.RESTServer, "create_application", lambda self: None)
    data_plane = Mock()
    model_repo_ext = Mock()

    rs = rest_mod.RESTServer(
        app="dummy:app",
        data_plane=data_plane,
        model_repository_extension=model_repo_ext,
        http_port=8080,
        event_loop=loop_value,
    )

    assert rs.config.loop == expected
