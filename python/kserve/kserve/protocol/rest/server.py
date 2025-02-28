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

import asyncio
import logging
import multiprocessing as mp
import os
import socket
from typing import Callable, Dict, List, Optional, Union

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.routing import APIRouter
from prometheus_client import REGISTRY, exposition
from timing_asgi import TimingClient, TimingMiddleware
from timing_asgi.integrations import StarletteScopeToName

from kserve.errors import (
    InferenceError,
    InvalidInput,
    ModelNotFound,
    ModelNotReady,
    ServerNotLive,
    ServerNotReady,
    UnsupportedProtocol,
    generic_exception_handler,
    inference_error_handler,
    invalid_input_handler,
    model_not_found_handler,
    model_not_ready_handler,
    not_implemented_error_handler,
    server_not_live_handler,
    server_not_ready_handler,
    unsupported_protocol_error_handler,
)
from kserve.logging import trace_logger, logger
from kserve.protocol.dataplane import DataPlane

from .v1_endpoints import register_v1_endpoints
from .v2_endpoints import register_v2_endpoints
from ..model_repository_extension import ModelRepositoryExtension


async def metrics_handler(request: Request) -> Response:
    encoder, content_type = exposition.choose_encoder(request.headers.get("accept"))
    return Response(content=encoder(REGISTRY), headers={"content-type": content_type})


class PrintTimings(TimingClient):
    def timing(self, metric_name, timing, tags):
        trace_logger.info(f"{metric_name}: {timing} {tags}")


class RESTServer:
    def __init__(
        self,
        app: FastAPI,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension,
        http_port: int,
        log_config: Optional[Union[str, Dict]] = None,
        access_log_format: Optional[str] = None,
        workers: int = 1,
        grace_period: int = 30,
    ):
        self.app = app
        self.dataplane = data_plane
        self.model_repository_extension = model_repository_extension
        self.access_log_format = access_log_format
        self._config = uvicorn.Config(
            "kserve.model_server:app",
            host="0.0.0.0",
            port=http_port,
            workers=workers,
            log_config=log_config,
            timeout_graceful_shutdown=grace_period,
            loop="asyncio",
        )
        self._server = uvicorn.Server(self._config)
        self._multiprocess_server = None

    def _register_endpoints(self):
        root_router = APIRouter()
        root_router.add_api_route(r"/", self.dataplane.live)
        root_router.add_api_route(r"/metrics", metrics_handler, methods=["GET"])
        self.app.include_router(root_router)
        register_v1_endpoints(self.app, self.dataplane, self.model_repository_extension)
        register_v2_endpoints(self.app, self.dataplane, self.model_repository_extension)
        # Register OpenAI endpoints if any of the models in the registry implement the OpenAI interface
        # This adds /openai/v1/completions and /openai/v1/chat/completions routes to the
        # REST server.
        try:
            from kserve.protocol.rest.openai.config import (
                maybe_register_openai_endpoints,
            )

            maybe_register_openai_endpoints(self.app, self.dataplane.model_registry)
            logger.info("OpenAI endpoints registered")
        except ImportError:
            logger.info("OpenAI endpoints not registered")
            pass

    def _add_exception_handlers(self):
        self.app.add_exception_handler(InvalidInput, invalid_input_handler)
        self.app.add_exception_handler(InferenceError, inference_error_handler)
        self.app.add_exception_handler(ModelNotFound, model_not_found_handler)
        self.app.add_exception_handler(ModelNotReady, model_not_ready_handler)
        self.app.add_exception_handler(
            NotImplementedError, not_implemented_error_handler
        )
        self.app.add_exception_handler(
            UnsupportedProtocol, unsupported_protocol_error_handler
        )
        self.app.add_exception_handler(ServerNotLive, server_not_live_handler)
        self.app.add_exception_handler(ServerNotReady, server_not_ready_handler)
        self.app.add_exception_handler(Exception, generic_exception_handler)

    def _add_middlewares(self):
        self.app.add_middleware(
            TimingMiddleware,
            client=PrintTimings(),
            metric_namer=StarletteScopeToName(
                prefix="kserve.io", starlette_app=self.app
            ),
        )

        # More context in https://github.com/encode/uvicorn/pull/947
        # At the time of writing the ASGI specs are not clear when it comes
        # to change the access log format, and hence the Uvicorn upstream devs
        # chose to create a custom middleware for this.
        # The allowed log format is specified in https://github.com/Kludex/asgi-logger#usage
        if self.access_log_format:
            from asgi_logger import AccessLoggerMiddleware

            # As indicated by the asgi-logger docs, we need to clear/unset
            # any setting for uvicorn.access to avoid log duplicates.
            logging.getLogger("uvicorn.access").handlers = []
            self.app.add_middleware(
                AccessLoggerMiddleware, format=self.access_log_format
            )
            # The asgi-logger settings don't set propagate to False,
            # so we get duplicates if we don't set it explicitly.
            logging.getLogger("access").propagate = False

    def create_application(self):
        self._add_middlewares()
        self._register_endpoints()
        self._add_exception_handlers()

    async def start(self):
        self.create_application()
        logger.info("Starting uvicorn with %s workers", self._config.workers)
        if self._config.workers > 1:
            self._multiprocess_server = RESTMultiProcess(self._server.run, self._config)
            await self._multiprocess_server.run()
        else:
            await self._server.serve()

    def stop(self, sig: int = None):
        # Uvicorn registers its own signal handlers, so we don't need to do anything here for single process mode
        if self._multiprocess_server:
            self._multiprocess_server.should_exit.set()


class RESTMultiProcess:
    def __init__(
        self, target: Callable[[list[socket.socket]], None], config: uvicorn.Config
    ):
        self._target = target
        self._config = config
        self._sockets: List[socket.socket] = []
        self._processes: List[mp.Process] = []
        self.should_exit = asyncio.Event()
        self.mp_ctx = mp.get_context("spawn")

    def init_processes(self, sockets: List[socket.socket]):
        for _ in range(self._config.workers):
            p = self.mp_ctx.Process(target=self._target, args=[sockets])
            self._processes.append(p)
            p.start()

    async def run(self):
        logger.info("Started parent process [%s]", os.getpid())
        self._sockets = [self._config.bind_socket()]
        self.init_processes(self._sockets)
        # Blocks until the parent process is ready to exit
        await self.keep_subprocess_alive()
        # Propagate signal to all child processes and wait for termination
        await self.terminate_all()

    async def keep_subprocess_alive(self):
        while not self.should_exit.is_set():
            for idx, p in enumerate(self._processes):
                if self.should_exit.is_set():
                    return  # parent process is exiting, no need to keep subprocess alive

                if not p.is_alive():
                    logger.info("Child process [%s] died", p.pid)
                    new_p = self.mp_ctx.Process(
                        target=self._target, args=[self._sockets]
                    )
                    new_p.start()
                    self._processes[idx] = new_p
                    logger.info("Started new child process [%s]", new_p.pid)
            await asyncio.sleep(0.5)  # Check every 0.5 seconds

    async def _wait_for_process(self, p: mp.Process):
        while p.is_alive():
            await asyncio.sleep(0.1)

    async def _wait_for_process_termination(self, p: mp.Process, grace_period: int):
        try:
            await asyncio.wait_for(self._wait_for_process(p), timeout=grace_period)
            logger.info("Terminated child process [%s]", p.pid)
        except asyncio.TimeoutError:
            logger.warning(
                "Child process [%s] took too long to terminate, forcing kill", p.pid
            )
            p.kill()

    async def terminate_all(self):
        """Propagate signal to all child processes and wait for termination."""
        for p in self._processes:
            if p.is_alive():
                p.terminate()  # Send SIGTERM (Unix) or TerminateProcess (Windows)

        # Wait for processes to exit
        await asyncio.gather(
            *(
                self._wait_for_process_termination(
                    p, self._config.timeout_graceful_shutdown
                )
                for p in self._processes
            )
        )
