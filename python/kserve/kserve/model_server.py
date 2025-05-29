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
import signal
import sys
from importlib import metadata
from typing import Any, Callable, Dict, List, Optional

from fastapi import FastAPI
from fastapi.responses import ORJSONResponse

from . import logging
from .constants.constants import (
    DEFAULT_HTTP_PORT,
    DEFAULT_GRPC_PORT,
    MAX_GRPC_MESSAGE_LENGTH,
    FASTAPI_APP_IMPORT_STRING,
)
from .errors import NoModelReady
from .logging import logger
from .model import BaseKServeModel, PredictorConfig
from .model_repository import ModelRepository
from .protocol.dataplane import DataPlane
from .protocol.grpc.server import GRPCServer
from .protocol.model_repository_extension import ModelRepositoryExtension
from .protocol.rest.server import RESTServer
from .protocol.rest.multiprocess.server import RESTServerMultiProcess
from .utils import utils
from .utils.inference_client_factory import InferenceClientFactory


parser = argparse.ArgumentParser(
    add_help=False, formatter_class=argparse.ArgumentDefaultsHelpFormatter
)
# Model Server Arguments: The arguments are passed to the kserve.ModelServer object
parser.add_argument(
    "--http_port",
    default=DEFAULT_HTTP_PORT,
    type=int,
    help="The HTTP Port listened to by the model server.",
)
parser.add_argument(
    "--grpc_port",
    default=DEFAULT_GRPC_PORT,
    type=int,
    help="The GRPC Port listened to by the model server.",
)
parser.add_argument(
    "--workers",
    default=1,
    type=int,
    help="The number of uvicorn workers for multi-processing.",
)
parser.add_argument(
    "--max_threads",
    default=4,
    type=int,
    help="The max number of gRPC processing threads.",
)
parser.add_argument(
    "--max_asyncio_workers",
    default=None,
    type=int,
    help="The max number of asyncio workers to spawn.",
)
parser.add_argument(
    "--enable_grpc",
    default=True,
    type=lambda x: utils.strtobool(x),
    help="Enable gRPC for the model server.",
)
parser.add_argument(
    "--enable_docs_url",
    default=False,
    type=lambda x: utils.strtobool(x),
    help="Enable docs url '/docs' to display Swagger UI.",
)
parser.add_argument(
    "--enable_latency_logging",
    default=True,
    type=lambda x: utils.strtobool(x),
    help="Enable a log line per request with preprocess/predict/postprocess latency metrics.",
)
parser.add_argument(
    "--configure_logging",
    default=True,
    type=lambda x: utils.strtobool(x),
    help="Enable to configure KServe and Uvicorn logging.",
)
parser.add_argument(
    "--log_config_file",
    default=None,
    type=str,
    help="File path containing UvicornServer's log config. Needs to be a yaml or json file.",
)
parser.add_argument(
    "--access_log_format",
    default=None,
    type=str,
    help="The asgi access logging format. It allows to override only the `uvicorn.access`'s format configuration "
    "with a richer set of fields",
)

# Model arguments: The arguments are passed to the kserve.Model object
parser.add_argument(
    "--model_name",
    default="model",
    type=str,
    help="The name of the model used on the endpoint path.",
)
parser.add_argument(
    "--predictor_host",
    default=None,
    type=str,
    help="The host name used for calling to the predictor from transformer.",
)
# For backwards compatibility.
parser.add_argument(
    "--protocol",
    default="v1",
    type=str,
    choices=["v1", "v2", "grpc-v2"],
    help="The inference protocol used for calling to the predictor from transformer. "
    "Deprecated and replaced by --predictor_protocol",
)
parser.add_argument(
    "--predictor_protocol",
    default="v1",
    type=str,
    choices=["v1", "v2", "grpc-v2"],
    help="The inference protocol used for calling to the predictor from transformer.",
)
parser.add_argument(
    "--predictor_use_ssl",
    default=False,
    type=lambda x: utils.strtobool(x),
    help="Use ssl for the http connection to the predictor.",
)
parser.add_argument(
    "--predictor_request_timeout_seconds",
    default=600,
    type=int,
    help="The timeout seconds for the request sent to the predictor.",
)
parser.add_argument(
    "--grpc_max_send_message_length",
    default=MAX_GRPC_MESSAGE_LENGTH,
    type=int,
    help="The max message length for gRPC send message.",
)
parser.add_argument(
    "--grpc_max_receive_message_length",
    default=MAX_GRPC_MESSAGE_LENGTH,
    type=int,
    help="The max message length for gRPC receive message.",
)
parser.add_argument(
    "--predictor_request_retries",
    default=0,
    type=int,
    help="The number of retries if predictor request fails. Defaults to 0.",
)
parser.add_argument(
    "--enable_predictor_health_check",
    action="store_true",
    help="The Transformer will perform readiness check for the predictor in addition to "
    "its health check. By default it is disabled.",
)
args, _ = parser.parse_known_args()

app = FastAPI(
    title="KServe ModelServer",
    version=metadata.version("kserve"),
    docs_url="/docs" if args.enable_docs_url else None,
    redoc_url=None,
    default_response_class=ORJSONResponse,
)


class ModelServer:
    def __init__(
        self,
        http_port: int = args.http_port,
        grpc_port: int = args.grpc_port,
        workers: int = args.workers,
        max_threads: int = args.max_threads,
        max_asyncio_workers: int = args.max_asyncio_workers,
        registered_models: Optional[ModelRepository] = None,
        enable_grpc: bool = args.enable_grpc,
        enable_docs_url: bool = args.enable_docs_url,
        enable_latency_logging: bool = args.enable_latency_logging,
        access_log_format: str = args.access_log_format,
        grace_period: int = 30,
        predictor_config: Optional[PredictorConfig] = None,
    ):
        """KServe ModelServer Constructor

        Args:
            http_port: HTTP port. Default: ``8080``.
            grpc_port: GRPC port. Default: ``8081``.
            workers: Number of uvicorn workers. Default: ``1``.
            max_threads: Max number of gRPC processing threads. Default: ``4``
            max_asyncio_workers: Max number of AsyncIO threads. Default: ``None``
            registered_models: A optional Model repository with registered models.
            enable_grpc: Whether to turn on grpc server. Default: ``True``
            enable_docs_url: Whether to turn on ``/docs`` Swagger UI. Default: ``False``.
            enable_latency_logging: Whether to log latency metric. Default: ``True``.
            access_log_format: Format to set for the access log (provided by asgi-logger). Default: ``None``.
                               it allows to override only the `uvicorn.access`'s format configuration with a richer
                               set of fields (output hardcoded to `stdout`). This limitation is currently due to the
                               ASGI specs that don't describe how access logging should be implemented in detail
                               (please refer to this Uvicorn
                               [github issue](https://github.com/encode/uvicorn/issues/527) for more info).
            grace_period: The grace period in seconds to wait for the server to stop. Default: ``30``.
            predictor_config: Optional configuration for the predictor. Default: ``None``.
        """
        self.registered_models = (
            ModelRepository() if registered_models is None else registered_models
        )
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.workers = workers
        self.max_threads = max_threads
        self.max_asyncio_workers = max_asyncio_workers
        self.enable_grpc = enable_grpc
        self.enable_docs_url = enable_docs_url
        self.enable_latency_logging = enable_latency_logging
        self.model_repository_extension = ModelRepositoryExtension(
            model_registry=self.registered_models
        )
        self.grace_period = grace_period
        if args.configure_logging:
            # If the logger does not have any handlers, then the logger is not configured.
            # For backward compatibility, we configure the logger here.
            if len(logger.handlers) == 0:
                logging.configure_logging(args.log_config_file)
        self.access_log_format = access_log_format
        self._custom_exception_handler = None
        if predictor_config is not None:
            _predictor_config = predictor_config
        else:
            _predictor_config = PredictorConfig(
                predictor_host=args.predictor_host,
                predictor_protocol=args.predictor_protocol,
                predictor_use_ssl=args.predictor_use_ssl,
                predictor_request_timeout_seconds=args.predictor_request_timeout_seconds,
                predictor_request_retries=args.predictor_request_retries,
                predictor_health_check=args.enable_predictor_health_check,
            )
        self.dataplane = DataPlane(
            model_registry=self.registered_models, predictor_config=_predictor_config
        )
        self._rest_server = None
        self._rest_multiprocess_server = None
        self._grpc_server = None
        self.servers = []

    def setup_event_loop(self):
        loop = asyncio.get_event_loop()
        if self._custom_exception_handler is None:
            loop.set_exception_handler(self.default_exception_handler)
        else:
            loop.set_exception_handler(self._custom_exception_handler)

        if self.max_asyncio_workers is None:
            # formula as suggest in https://bugs.python.org/issue35279
            self.max_asyncio_workers = min(32, utils.cpu_count() + 4)
        logger.info(f"Setting max asyncio worker threads as {self.max_asyncio_workers}")
        loop.set_default_executor(
            concurrent.futures.ThreadPoolExecutor(max_workers=self.max_asyncio_workers)
        )

    def register_signal_handler(self):
        if sys.platform == "win32":
            sig_list = [signal.SIGINT, signal.SIGTERM, signal.SIGBREAK]
        else:
            sig_list = [signal.SIGINT, signal.SIGTERM, signal.SIGQUIT]

        for sig in sig_list:
            signal.signal(sig, lambda sig, frame: self.stop(sig))

    async def _serve(self):
        await asyncio.gather(*self.servers)

    def start(self, models: List[BaseKServeModel]) -> None:
        """Start the model server with a set of registered models.

        Args:
            models: a list of models to register to the model server.
        """
        self._register_and_check_atleast_one_model_is_ready(models)
        if self.workers > 1:
            self._rest_multiprocess_server = RESTServerMultiProcess(
                FASTAPI_APP_IMPORT_STRING,
                self.dataplane,
                self.model_repository_extension,
                self.http_port,
                access_log_format=self.access_log_format,
                workers=self.workers,
                grace_period=self.grace_period,
                log_config_file=args.log_config_file,
            )
            self.servers.append(self._rest_multiprocess_server.start())
        else:
            self._rest_server = RESTServer(
                FASTAPI_APP_IMPORT_STRING,
                self.dataplane,
                self.model_repository_extension,
                self.http_port,
                access_log_format=self.access_log_format,
                workers=self.workers,
                grace_period=self.grace_period,
            )
            self.servers.append(self._rest_server.start())
        if self.enable_grpc:
            self._grpc_server = GRPCServer(
                self.grpc_port,
                self.dataplane,
                self.model_repository_extension,
                kwargs=vars(args),
                grace_period=self.grace_period,
            )
            self.servers.append(self._grpc_server.start(self.max_threads))
        self.setup_event_loop()
        self.register_signal_handler()
        asyncio.run(self._serve())

    def stop(self, sig: int):
        """Stop the instances of REST and gRPC model servers.

        Args:
            sig: The signal to stop the server.
        """

        async def shutdown():
            await InferenceClientFactory().close()
            logger.info("Stopping the model server")
            if self._rest_multiprocess_server:
                logger.info("Stopping the rest server")
                await self._rest_multiprocess_server.stop(sig)
            if self._grpc_server:
                logger.info("Stopping the grpc server")
                await self._grpc_server.stop(sig)

        asyncio.create_task(shutdown())
        for model_name in list(self.registered_models.get_models().keys()):
            self.registered_models.unload(model_name)

    def register_exception_handler(
        self,
        handler: Callable[[asyncio.events.AbstractEventLoop, Dict[str, Any]], None],
    ):
        """Add a custom handler as the event loop exception handler.

        If a handler is not provided, the default exception handler will be set.

        handler should be a callable object, it should have a signature matching '(loop, context)', where 'loop'
        will be a reference to the active event loop, 'context' will be a dict object (see `call_exception_handler()`
        documentation for details about context).
        """
        self._custom_exception_handler = handler

    def default_exception_handler(
        self, loop: asyncio.events.AbstractEventLoop, context: Dict[str, Any]
    ):
        """Default exception handler for event loop.

        This is called when an exception occurs and no exception handler is set.
        This can be called by a custom exception handler that wants to defer to the default handler behavior.
        """
        if "exception" in context:
            logger.error(f"Caught exception: {context.get('exception')}")
        logger.error(f"message: {context.get('message')}")
        loop.default_exception_handler(context)

    def register_model(self, model: BaseKServeModel, name: Optional[str] = None):
        """Register a model to the model server.

        Args:
            model: The model object.
            name: The name of the model. If not provided, the model's name will be used. This can be used to provide
                additional names for the same model.
        """
        if not model.name:
            raise ValueError("Failed to register model, model.name must be provided.")
        name = name or model.name
        self.registered_models.update(model, name)
        logger.info("Registering model: %s", name)

    def _register_and_check_atleast_one_model_is_ready(
        self, models: List[BaseKServeModel]
    ):
        if isinstance(models, list):
            at_least_one_model_ready = False
            for model in models:
                if isinstance(model, BaseKServeModel):
                    if model.ready:
                        at_least_one_model_ready = True
                        self.register_model(model)
                        # pass whether to log request latency into the model
                        model.enable_latency_logging = self.enable_latency_logging
                    model.start()
                    if model.engine:
                        self.servers.append(model.start_engine())
                else:
                    raise RuntimeError("Model type should be 'BaseKServeModel'")
            if not at_least_one_model_ready and models:
                raise NoModelReady(models)
        else:
            raise RuntimeError("Unknown model collection type")
