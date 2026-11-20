# Copyright 2026 The KServe Authors.
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
import os
from pathlib import Path

import pytest

from kserve import ModelRepository
from kserve import context as kserve_context
from kserve.constants.constants import FASTAPI_APP_IMPORT_STRING
from kserve.predictor_config import PredictorConfig
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest.multiprocess.server import RESTServerMultiProcess

PREDICTOR_HOST = "predictor.example.com:8080"


def record_predictor_host(sockets, out_dir):
    """Runs inside a spawned worker and records the predictor host it can see."""
    config = kserve_context.get_predictor_config()
    host = config.predictor_host if config is not None else "<unset>"
    Path(out_dir, str(os.getpid())).write_text(host)


@pytest.fixture
def predictor_config():
    previous = kserve_context.get_predictor_config()
    config = PredictorConfig(predictor_host=PREDICTOR_HOST)
    # ModelServer sets the config in the parent process before it starts the workers.
    kserve_context.set_predictor_config(config)
    yield config
    kserve_context.set_predictor_config(previous)


def make_server(tmp_path, workers, log_config_file=None):
    model_repository = ModelRepository()
    server = RESTServerMultiProcess(
        FASTAPI_APP_IMPORT_STRING,
        DataPlane(model_registry=model_repository),
        ModelRepositoryExtension(model_registry=model_repository),
        8080,
        workers=workers,
        log_config_file=log_config_file,
    )
    # Replace the uvicorn entry point with a function that reports what the worker sees.
    server._rest_server.run = functools.partial(
        record_predictor_host, out_dir=str(tmp_path)
    )
    return server


def join_workers(server):
    for process in server._processes:
        process._process.join(timeout=120)
        assert process._process.exitcode == 0


def recorded_hosts(tmp_path):
    return [path.read_text() for path in sorted(tmp_path.iterdir())]


def test_workers_receive_predictor_config(tmp_path, predictor_config):
    server = make_server(tmp_path, workers=2)

    server.init_processes([])
    join_workers(server)

    assert recorded_hosts(tmp_path) == [PREDICTOR_HOST] * 2


@pytest.mark.asyncio
async def test_restarted_worker_receives_predictor_config(tmp_path, predictor_config):
    log_config = tmp_path / "log_config.yaml"
    log_config.write_text("version: 1\ndisable_existing_loggers: false\n")
    out_dir = tmp_path / "out"
    out_dir.mkdir()
    server = make_server(out_dir, workers=1, log_config_file=str(log_config))

    server.init_processes([])
    join_workers(server)
    # The first worker has exited, so the supervisor treats it as dead and restarts it.
    await server.keep_subprocess_alive([])
    join_workers(server)

    assert recorded_hosts(out_dir) == [PREDICTOR_HOST] * 2
    assert server._processes[0]._log_config_file == str(log_config)
