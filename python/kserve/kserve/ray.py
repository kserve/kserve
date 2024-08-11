# Copyright 2023 The KServe Authors.
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

from kserve.logging import logger

try:
    from ray.serve.handle import DeploymentHandle, DeploymentResponse
except ImportError:
    logger.error(
        "Ray dependency is missing. Install Ray Serve with: pip install kserve[ray]"
    )
    raise
from typing import cast, Dict, Union, Optional
from cloudevents.http import CloudEvent

from .model import InferenceModel, InferenceVerb, InferReturnType
from .protocol.infer_type import InferRequest


class RayModel(InferenceModel):
    """
    Wrapper for a model that is deployed with Ray. All calls are delegated to the DeploymentHandle.
    """

    def __init__(self, name: str, handle: DeploymentHandle):
        super().__init__(name)
        self._handle = handle

    def __call__(
        self,
        body: Union[Dict, CloudEvent, InferRequest],
        headers: Optional[Dict[str, str]] = None,
        verb: InferenceVerb = InferenceVerb.PREDICT,
    ) -> InferReturnType:
        return cast(DeploymentResponse, self._handle.remote(body, headers=headers))

    def load(self):
        self._handle.load.remote()
        self.ready = True

    async def get_input_types(self):
        return await self._handle.get_input_types.remote()

    async def get_output_types(self):
        return await self._handle.get_output_types.remote()
