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

from typing import Dict

try:
    from ray import serve as rayserve
    from ray.serve.deployment import Deployment
    from .ray_model import RayServeModel
except ImportError:
    raise ImportError("Missing optional dependency 'ray.serve'. "
                      "To use KServe with Ray Serve run pip install kserve[ray-serve]")


from .model_server import ModelServer


class RayModelServer(ModelServer):
    """Ray ModelServer
    """

    def start(self, models: Dict[str, Deployment]) -> None:
        _models = []
        if all([isinstance(v, Deployment) for v in models.values()]):
            # TODO: make this port number a variable
            rayserve.start(detached=True, http_options={"host": "0.0.0.0", "port": 9071})
            for key in models:
                models[key].deploy()
                handle = models[key].get_handle()
                _models.append(RayServeModel(name=key, ray_handle=handle))
        else:
            raise RuntimeError("Model type should be RayServe Deployment")
        super().start(_models)
