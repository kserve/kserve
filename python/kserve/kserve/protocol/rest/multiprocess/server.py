# Copyright 2025 The KServe Authors.
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
import multiprocessing as mp
import os
import signal
from socket import socket
import threading
from typing import Any, Callable, List, Optional, Union

from kserve import logging
from kserve.logging import logger
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest.server import RESTServer

mp.allow_connection_pickling()
spawn = mp.get_context("spawn")


class RESTServerProcess:
    def __init__(
        self,
        sockets: list[socket],
        target: Callable[[list[socket]], None],
        log_config_file: Optional[str] = None,
    ) -> None:
        self._real_target = target
        # Pipe is used to identify if the process is responsive.
        # A ping request is sent from the parent_conn and is received by the child_conn which in turn responds with a pong.
        # If the request times out, then the process is considered unresponsive. We then kill the process and recreate it.
        self._parent_conn, self._child_conn = mp.Pipe()
        self._process = spawn.Process(target=self.target, args=[sockets])
        self._log_config_file = log_config_file

    def ping(self, timeout: float = 5) -> bool:
        self._parent_conn.send(b"ping")
        if self._parent_conn.poll(timeout):
            self._parent_conn.recv()
            return True
        return False

    def pong(self) -> None:
        self._child_conn.recv()
        self._child_conn.send(b"pong")

    def always_pong(self) -> None:
        while True:
            self.pong()

    def target(self, sockets: list[socket]) -> Any:
        if os.name == "nt":
            # Windows doesn't support SIGTERM, so we use SIGBREAK instead.
            # And then we raise SIGTERM when SIGBREAK is received.
            # https://learn.microsoft.com/zh-cn/cpp/c-runtime-library/reference/signal?view=msvc-170
            signal.signal(
                signal.SIGBREAK,
                lambda sig, frame: signal.raise_signal(signal.SIGTERM),
            )
        # Configure logging for every child
        logging.configure_logging(self._log_config_file)
        threading.Thread(target=self.always_pong, daemon=True).start()
        try:
            return self._real_target(sockets=sockets)
        except KeyboardInterrupt:
            # Suppress side-effects of signal propagation
            # the parent process already expects us to end, so no vital information is lost
            # https://github.com/encode/uvicorn/pull/2317
            pass

    def is_alive(self, timeout: float = 5) -> bool:
        if not self._process.is_alive():
            return False

        return self.ping(timeout)

    def start(self) -> None:
        self._process.start()

    def terminate(self) -> None:
        if self._process.exitcode is None:  # Process is still running
            assert self._process.pid is not None
            if os.name == "nt":
                # Windows doesn't support SIGTERM.
                # So send SIGBREAK, and then in process raise SIGTERM.
                os.kill(self._process.pid, signal.CTRL_BREAK_EVENT)
            else:
                os.kill(self._process.pid, signal.SIGTERM)
        self._parent_conn.close()
        self._child_conn.close()

    def kill(self) -> None:
        # In Windows, the method will call `TerminateProcess` to kill the process.
        # In Unix, the method will send SIGKILL to the process.
        self._process.kill()

    async def wait_for_termination(self, grace_period: Optional[int] = None):
        """Wait for the process to terminate. When a timeout occurs,
        it cancels the task and raises TimeoutError."""

        async def _wait_for_process():
            while self._process.exitcode is None:
                await asyncio.sleep(0.1)

        await asyncio.wait_for(_wait_for_process(), timeout=grace_period)

    @property
    def pid(self) -> Union[int, None]:
        return self._process.pid


class RESTServerMultiProcess:
    def __init__(
        self,
        app: str,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension,
        http_port: int,
        access_log_format: Optional[str] = None,
        workers: int = 1,
        grace_period: int = 30,
        log_config_file: Optional[str] = None,
    ) -> None:
        self.log_config_file = log_config_file
        self._rest_server = RESTServer(
            app,
            data_plane,
            model_repository_extension,
            http_port,
            access_log_format,
            workers,
            grace_period,
        )
        self._processes: List[RESTServerProcess] = []
        self.should_exit = asyncio.Event()

    def init_processes(self, sockets: List[socket]) -> None:
        for _ in range(self._rest_server.config.workers):
            p = RESTServerProcess(
                sockets,
                self._rest_server.run,
                log_config_file=self.log_config_file,
            )
            self._processes.append(p)
            p.start()
            logger.info("Started child process [%s]", p.pid)

    async def start(self) -> None:
        logger.info(
            "Starting uvicorn with %s workers", self._rest_server.config.workers
        )
        sockets = [self._rest_server.config.bind_socket()]
        logger.info("Started parent process [%s]", os.getpid())
        self.init_processes(sockets)
        # Blocks until the parent process is ready to exit
        while not self.should_exit.is_set():
            await asyncio.sleep(0.5)
            await self.keep_subprocess_alive(sockets)
        # Propagate signal to all child processes and wait for termination
        await self.terminate_all()

    async def keep_subprocess_alive(self, sockets: List[socket]) -> None:
        if self.should_exit.is_set():
            return  # parent process is exiting, no need to keep subprocess alive

        for idx, process in enumerate(self._processes):
            if not process.is_alive():
                process.kill()  # process is hung, kill it
                await process.wait_for_termination()
                if self.should_exit.is_set():
                    return

                logger.info("Child process [%s] died", process.pid)
                process = RESTServerProcess(
                    sockets,
                    self._rest_server.run,
                )
                process.start()
                self._processes[idx] = process
                logger.info("Started new child process [%s]", process.pid)

    async def terminate_all(self) -> None:
        """Propagate signal to all child processes and wait for termination."""
        for p in self._processes:
            p.terminate()

        async def force_terminate(process) -> None:
            try:
                await process.wait_for_termination(
                    self._rest_server.config.timeout_graceful_shutdown
                )
                logger.info("Terminated child process [%s]", process.pid)
            except asyncio.TimeoutError:
                logger.warning(
                    "Child process [%s] took too long to terminate, force terminating",
                    process.pid,
                )
                process.kill()

        await asyncio.gather(*(force_terminate(p) for p in self._processes))
        logger.info("Stopping parent process [%s]", os.getpid())

    async def stop(self, sig: Optional[int] = None) -> None:
        self.should_exit.set()
