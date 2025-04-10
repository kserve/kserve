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

import os
import subprocess
import sys
import time
import requests
from typing import Dict, List, Optional
import openai
from vllm.utils import FlexibleArgumentParser
from vllm.entrypoints.openai.cli_args import make_arg_parser
from vllm.engine.arg_utils import AsyncEngineArgs
from vllm.model_executor.model_loader.loader import get_model_loader


class RemoteOpenAIServer:
    DUMMY_API_KEY = "token-abc123"  # vLLM's OpenAI server does not need API key

    def __init__(
        self,
        model: str,
        model_name: str,
        vllm_serve_args: List[str],
        *,
        env_dict: Optional[Dict[str, str]] = None,
        max_wait_seconds: Optional[float] = None,
    ) -> None:

        parser = FlexibleArgumentParser(description="huggingface server")
        parser = make_arg_parser(parser)
        args = parser.parse_args(["--model", model, *vllm_serve_args])
        model_id = "--model_id=" + model
        model_name = "--model_name=" + model_name
        self.host = str(args.host or "localhost")
        self.port = 8080

        # download the model before starting the server to avoid timeout
        is_local = os.path.isdir(model)
        if not is_local:
            engine_args = AsyncEngineArgs.from_cli_args(args)
            model_config = engine_args.create_model_config()
            load_config = engine_args.create_load_config()

            model_loader = get_model_loader(load_config)
            model_loader.download_model(model_config)

        env = os.environ.copy()
        # the current process might initialize cuda,
        # to be safe, we should use spawn method
        env["VLLM_WORKER_MULTIPROC_METHOD"] = "spawn"
        if env_dict is not None:
            env.update(env_dict)
        self.proc = subprocess.Popen(
            [
                "python",
                "-m",
                "huggingfaceserver",
                model_id,
                model_name,
                "--backend=vllm",
                *vllm_serve_args,
            ],
            env=env,
            stdout=sys.stdout,
            stderr=sys.stderr,
        )
        max_wait_seconds = max_wait_seconds or 240
        self._wait_for_server(
            url=self.url_for("v2/health/ready"), timeout=max_wait_seconds
        )

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_value, traceback):
        self.proc.terminate()
        try:
            self.proc.wait(8)
        except subprocess.TimeoutExpired:
            # force kill if needed
            self.proc.kill()

    def _wait_for_server(self, *, url: str, timeout: float):
        # run health check
        start = time.time()
        while True:
            try:
                time.sleep(60)
                if requests.get(url).status_code == 200:
                    break
            except Exception:
                # this exception can only be raised by requests.get,
                # which means the server is not ready yet.
                # the stack trace is not useful, so we suppress it
                # by using `raise from None`.
                result = self.proc.poll()
                if result is not None and result != 0:
                    raise RuntimeError("Server exited unexpectedly.") from None

                if time.time() - start > timeout:
                    raise RuntimeError("Server failed to start in time.") from None

    @property
    def url_root(self) -> str:
        return f"http://{self.host}:{self.port}"

    def url_for(self, *parts: str) -> str:
        return self.url_root + "/" + "/".join(parts)

    def get_client(self):
        return openai.OpenAI(
            base_url=self.url_for("openai/v1"),
            api_key=self.DUMMY_API_KEY,
        )

    def get_async_client(self, **kwargs):
        return openai.AsyncOpenAI(
            base_url=self.url_for("openai/v1"),
            api_key=self.DUMMY_API_KEY,
            max_retries=0,
            **kwargs,
        )
