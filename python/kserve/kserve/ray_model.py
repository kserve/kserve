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

from typing import Dict, List, Union, Optional
from ray.serve.handle import RayServeHandle
from .protocol.infer_type import InferRequest
from cloudevents.http import CloudEvent

from .model import KServeModel, ModelType


class RayServeModel(KServeModel):
    handle: RayServeHandle

    def __init__(self, name: str, ray_handle: RayServeHandle):
        super().__init__(name)
        self.handle = ray_handle
        self.ready = True

    async def __call__(self, body: Union[Dict, CloudEvent, InferRequest],
                       model_type: ModelType = ModelType.PREDICTOR,
                       headers: Optional[Dict[str, str]] = None) -> Dict:
        res = await self.handle.remote(body, model_type=model_type, headers=headers)
        return res

    async def get_input_types(self) -> List[Dict]:
        return await self.handle.get_input_types.remote()

    async def get_output_types(self) -> List[Dict]:
        return await self.handle.get_output_types.remote()
