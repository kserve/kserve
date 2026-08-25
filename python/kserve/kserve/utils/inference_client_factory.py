# Copyright 2024 The KServe Authors.
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

from typing import Optional
from ..inference_client import InferenceGRPCClient, InferenceRESTClient, RESTConfig
from .utils import is_v2


class InferenceClientFactory:
    _instance = None
    _grpc_client: Optional[InferenceGRPCClient] = None
    _rest_v1_client: Optional[InferenceRESTClient] = None
    _rest_v2_client: Optional[InferenceRESTClient] = None

    def __new__(cls):
        if cls._instance is None:
            cls._instance = super(InferenceClientFactory, cls).__new__(cls)
        return cls._instance

    def get_grpc_client(self, url: str, **kwargs) -> InferenceGRPCClient:
        if self._grpc_client is None:
            self._grpc_client = InferenceGRPCClient(url, **kwargs)
        return self._grpc_client

    def get_rest_client(self, config: RESTConfig = None) -> InferenceRESTClient:
        if config and is_v2(config.protocol):
            if self._rest_v2_client is None:
                self._rest_v2_client = InferenceRESTClient(config)
            return self._rest_v2_client
        if self._rest_v1_client is None:
            self._rest_v1_client = InferenceRESTClient(config)
        return self._rest_v1_client

    async def close(self):
        if self._grpc_client is not None:
            await self._grpc_client.close()
        if self._rest_v1_client is not None:
            await self._rest_v1_client.close()
        if self._rest_v2_client is not None:
            await self._rest_v2_client.close()
