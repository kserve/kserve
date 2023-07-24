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

import argparse
import asyncio
import concurrent.futures
import multiprocessing
import signal
import socket
from multiprocessing import Process
from typing import Dict, List, Optional, Union

from ray import serve as rayserve
from ray.serve.api import Deployment
from ray.serve.handle import RayServeHandle

from .logging import KSERVE_LOG_CONFIG, logger
from .model import Model
from .model_repository import ModelRepository
from .protocol.dataplane import DataPlane
from .protocol.grpc.server import GRPCServer
from .protocol.model_repository_extension import ModelRepositoryExtension
from .protocol.rest.server import UvicornServer
from .utils import utils

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument("--http_port", default=DEFAULT_HTTP_PORT, type=int,
                    help="The HTTP Port listened to by the model server.")
parser.add_argument("--grpc_port", default=DEFAULT_GRPC_PORT, type=int,
                    help="The GRPC Port listened to by the model server.")
parser.add_argument("--workers", default=1, type=int,
                    help="The number of workers for multi-processing.")
parser.add_argument("--max_threads", default=4, type=int,
                    help="The number of max processing threads in each worker.")
parser.add_argument('--max_asyncio_workers', default=None, type=int,
                    help='Max number of asyncio workers to spawn')
parser.add_argument("--enable_grpc", default=True, type=lambda x: utils.strtobool(x),
                    help="Enable gRPC for the model server")
parser.add_argument("--enable_docs_url", default=False, type=lambda x: utils.strtobool(x),
                    help="Enable docs url '/docs' to display Swagger UI.")
parser.add_argument("--enable_latency_logging", default=True, type=lambda x: utils.strtobool(x),
                    help="Output a log per request with latency metrics.")
parser.add_argument("--configure_logging", default=True, type=lambda x: utils.strtobool(x),
                    help="Whether to configure KServe and Uvicorn logging")
parser.add_argument("--log_config_file", default=None, type=str,
                    help="File path containing UvicornServer's log config. Needs to be a yaml or json file.")
parser.add_argument("--access_log_format", default=None, type=str,
                    help="Format to set for the access log (provided by asgi-logger).")

args, _ = parser.parse_known_args()


class ModelServer:
    """KServe ModelServer

    Args:
        http_port (int): HTTP port. Default: ``8080``.
        grpc_port (int): GRPC port. Default: ``8081``.
        workers (int): Number of workers for uvicorn. Default: ``1``.
        max_threads (int): Max number of processing threads. Default: ``4``
        max_asyncio_workers (int): Max number of AsyncIO threads. Default: ``None``
        registered_models (ModelRepository): Model repository with registered models.
        enable_grpc (bool): Whether to turn on grpc server. Default: ``True``
        enable_docs_url (bool): Whether to turn on ``/docs`` Swagger UI. Default: ``False``.
        enable_latency_logging (bool): Whether to log latency metric. Default: ``True``.
        configure_logging (bool): Whether to configure KServe and Uvicorn logging. Default: ``True``.
        log_config (dict or str): File path or dict containing log config. Default: ``None``.
        access_log_format (string): Format to set for the access log (provided by asgi-logger). Default: ``None``
    """

    def __init__(self, http_port: int = args.http_port,
                 grpc_port: int = args.grpc_port,
                 workers: int = args.workers,
                 max_threads: int = args.max_threads,
                 max_asyncio_workers: int = args.max_asyncio_workers,
                 registered_models: ModelRepository = ModelRepository(),
                 enable_grpc: bool = args.enable_grpc,
                 enable_docs_url: bool = args.enable_docs_url,
                 enable_latency_logging: bool = args.enable_latency_logging,
                 configure_logging: bool = args.configure_logging,
                 log_config: Optional[Union[Dict, str]] = args.log_config_file,
                 access_log_format: str = args.access_log_format):
        self.registered_models = registered_models
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.workers = workers
        self.max_threads = max_threads
        self.max_asyncio_workers = max_asyncio_workers
        self.enable_grpc = enable_grpc
        self.enable_docs_url = enable_docs_url
        self.enable_latency_logging = enable_latency_logging
        self.dataplane = DataPlane(model_registry=registered_models)
        self.model_repository_extension = ModelRepositoryExtension(
            model_registry=self.registered_models)
        self._grpc_server = None
        if self.enable_grpc:
            self._grpc_server = GRPCServer(grpc_port, self.dataplane,
                                           self.model_repository_extension)

        # Logs can be passed as a path to a file or a dictConfig.
        # We rely on Uvicorn to configure the loggers for us.
        if configure_logging:
            self.log_config = log_config if log_config is not None else KSERVE_LOG_CONFIG
        else:
            # By setting log_config to None we tell Uvicorn not to configure logging
            self.log_config = None

        self.access_log_format = access_log_format

    def start(self, models: Union[List[Model], Dict[str, Deployment]]) -> None:
        if isinstance(models, list):
            for model in models:
                if isinstance(model, Model):
                    self.register_model(model)
                    # pass whether to log request latency into the model
                    model.enable_latency_logging = self.enable_latency_logging
                else:
                    raise RuntimeError("Model type should be 'Model'")
        elif isinstance(models, dict):
            if all([isinstance(v, Deployment) for v in models.values()]):
                # TODO: make this port number a variable
                rayserve.start(detached=True, http_options={"host": "0.0.0.0", "port": 9071})
                for key in models:
                    models[key].deploy()
                    handle = models[key].get_handle()
                    self.register_model_handle(key, handle)
            else:
                raise RuntimeError("Model type should be RayServe Deployment")
        else:
            raise RuntimeError("Unknown model collection types")

        if self.max_asyncio_workers is None:
            # formula as suggest in https://bugs.python.org/issue35279
            self.max_asyncio_workers = min(32, utils.cpu_count() + 4)
        logger.info(f"Setting max asyncio worker threads as {self.max_asyncio_workers}")
        asyncio.get_event_loop().set_default_executor(
            concurrent.futures.ThreadPoolExecutor(max_workers=self.max_asyncio_workers))

        async def serve():
            logger.info(f"Starting uvicorn with {self.workers} workers")
            loop = asyncio.get_event_loop()
            for sig in [signal.SIGINT, signal.SIGTERM, signal.SIGQUIT]:
                loop.add_signal_handler(
                    sig, lambda s=sig: asyncio.create_task(self.stop(sig=s))
                )
            if self.workers == 1:
                self._rest_server = UvicornServer(self.http_port, [],
                                                  self.dataplane, self.model_repository_extension,
                                                  self.enable_docs_url,
                                                  log_config=self.log_config,
                                                  access_log_format=self.access_log_format)
                await self._rest_server.run()
            else:
                # Since py38 MacOS/Windows defaults to use spawn for starting multiprocessing.
                # https://docs.python.org/3/library/multiprocessing.html#contexts-and-start-methods
                # Spawn does not work with FastAPI/uvicorn in multiprocessing mode, use fork for multiprocessing
                # https://github.com/tiangolo/fastapi/issues/1586
                serversocket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                serversocket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                serversocket.bind(('0.0.0.0', self.http_port))
                serversocket.listen(5)
                multiprocessing.set_start_method('fork')
                server = UvicornServer(self.http_port, [serversocket],
                                       self.dataplane, self.model_repository_extension,
                                       self.enable_docs_url, log_config=self.log_config,
                                       access_log_format=self.access_log_format)
                for _ in range(self.workers):
                    p = Process(target=server.run_sync)
                    p.start()

        async def servers_task():
            servers = [serve()]
            if self.enable_grpc:
                servers.append(self._grpc_server.start(self.max_threads))
            await asyncio.gather(*servers)

        asyncio.run(servers_task())

    async def stop(self, sig: Optional[int] = None):
        logger.info("Stopping the model server")
        if self._rest_server:
            logger.info("Stopping the rest server")
            await self._rest_server.stop()
        if self._grpc_server:
            logger.info("Stopping the grpc server")
            await self._grpc_server.stop(sig)

    def register_model_handle(self, name: str, model_handle: RayServeHandle):
        self.registered_models.update_handle(name, model_handle)
        logger.info("Registering model handle: %s", name)

    def register_model(self, model: Model):
        if not model.name:
            raise Exception(
                "Failed to register model, model.name must be provided.")
        self.registered_models.update(model)
        logger.info("Registering model: %s", model.name)
