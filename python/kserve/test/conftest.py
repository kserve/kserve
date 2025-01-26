# Copyright 2024 The KServe Authors.
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

import pytest
from kserve import ModelServer, ModelRepository
from kserve.protocol.rest.server import RESTServer
from kserve.model_server import app as kserve_app


@pytest.fixture(scope="session")
def server():
    server = ModelServer(registered_models=ModelRepository())
    rest_server = RESTServer(
        kserve_app,
        server.dataplane,
        server.model_repository_extension,
        http_port=8080,
    )
    rest_server.create_application()
    yield server
    kserve_app.routes.clear()
