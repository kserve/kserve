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

"""Small REST transformer used by the ISVC tracing E2E test.

The public endpoint accepts KServe V1 ``instances`` while the native vLLM
runtime accepts OpenAI completion requests.  Keeping that translation in the
test image exercises the normal transformer to predictor request path without
changing KServe's public protocol support.
"""

from __future__ import annotations

import argparse
from typing import Any

from kserve import Model, ModelServer, model_server
from kserve.model import append_forwardable_headers


class OpenAITraceTransformer(Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.ready = True

    async def predict(
        self,
        payload: dict[str, Any],
        headers: dict[str, str] | None = None,
    ) -> dict[str, list[str]]:
        instances = payload.get("instances")
        if not isinstance(instances, list) or len(instances) != 1:
            raise ValueError("expected exactly one V1 'instances' prompt")
        instance = instances[0]
        if isinstance(instance, str):
            prompt = instance
        elif isinstance(instance, dict) and isinstance(instance.get("prompt"), str):
            prompt = instance["prompt"]
        else:
            raise ValueError("expected instances[0] to contain a string prompt")

        response = await self._http_client.infer(
            f"{self.predictor_config.predictor_base_url.rstrip('/')}/v1/completions",
            {
                "model": self.name,
                "prompt": prompt,
                "max_tokens": 8,
                "temperature": 0,
            },
            headers=append_forwardable_headers(
                headers, {"Content-Type": "application/json"}
            ),
            is_graph_endpoint=True,
        )
        choices = response.get("choices", [])
        if not choices or not isinstance(choices[0].get("text"), str):
            raise ValueError(
                f"vLLM completion response did not contain text: {response!r}"
            )
        return {"predictions": [choices[0]["text"]]}


parser = argparse.ArgumentParser(parents=[model_server.parser])
args, _ = parser.parse_known_args()


if __name__ == "__main__":
    if args.configure_logging:
        from kserve import logging

        logging.configure_logging(args.log_config_file)
    model = OpenAITraceTransformer(args.model_name)
    ModelServer().start([model])
