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

import ssl
from typing import Union, List, Tuple, Any, Optional, Sequence, Mapping

import grpc
import httpx
from httpx import AsyncBaseTransport
from urllib3.util import Url

from .errors import UnsupportedProtocol
from .logging import logger
from .model import PredictorProtocol
from .protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest, ModelInferResponse, ServerReadyResponse, \
    ServerLiveResponse, ModelReadyResponse, ServerReadyRequest, ServerLiveRequest, ModelReadyRequest
from .protocol.grpc.grpc_predict_v2_pb2_grpc import GRPCInferenceServiceStub
from .protocol.infer_type import InferRequest, InferResponse
from .protocol.rest.v1_datamodels import PredictRequest, PredictResponse

from .logging import logger

class InferenceGRPCClient:
    """
    Asynchronous GRPC inference client. This feature is currently in alpha and may be subject to change.
    :param url: Inference server url as a string.
    :param verbose: (optional) A boolean to enable verbose logging. Defaults to False.
    :param use_ssl: (optional) A boolean value indicating whether to use an SSL-enabled channel (True) or not (False).
                    If creds provided the client will use SSL-enabled channel regardless of the specified value.
    :param root_certificates: (optional) Path to the PEM-encoded root certificates file as a string, or None to
                              retrieve them from a default location chosen by gRPC runtime. If creds provided this
                              will be ignored.
    :param private_key: (optional) Path to the PEM-encoded private key file as a string or None if no private key
                        should be used. If creds provided this will be ignored.
    :param certificate_chain: (optional) Path to the PEM-encoded certificate chain file as a string or None if no
                              certificate chain should be used. If creds provided this will be ignored.
    :param creds: (optional) A ChannelCredentials instance for secure channel communication.
    :param channel_args: (optional) An list of key-value pairs (channel_arguments in gRPC Core runtime) to configure
                         the channel.
    """

    def __init__(self,
                 url: str,
                 verbose: bool = False,
                 use_ssl: bool = False,
                 root_certificates: str = None,
                 private_key: str = None,
                 certificate_chain: str = None,
                 creds: grpc.ChannelCredentials = None,
                 channel_args: List[Tuple[str, Any]] = None):

        # Explicitly check "is not None" here to support passing an empty
        # list to specify setting no channel arguments.
        if channel_args is not None:
            channel_opt = channel_args
        else:
            # To specify custom channel_opt, see the channel_args parameter.
            channel_opt = [
                ("grpc.max_send_message_length", -1),
                ("grpc.max_receive_message_length", -1),
            ]

        if creds:
            self._channel = grpc.aio.secure_channel(url, creds, options=channel_opt)
        elif use_ssl:
            rc_bytes = pk_bytes = cc_bytes = None
            if root_certificates is not None:
                with open(root_certificates, "rb") as rc_fs:
                    rc_bytes = rc_fs.read()
            if private_key is not None:
                with open(private_key, "rb") as pk_fs:
                    pk_bytes = pk_fs.read()
            if certificate_chain is not None:
                with open(certificate_chain, "rb") as cc_fs:
                    cc_bytes = cc_fs.read()
            creds = grpc.ssl_channel_credentials(root_certificates=rc_bytes,
                                                 private_key=pk_bytes,
                                                 certificate_chain=cc_bytes)
            self._channel = grpc.aio.secure_channel(url, creds, options=channel_opt)
        else:
            self._channel = grpc.aio.insecure_channel(url, options=channel_opt)
        self._client_stub = GRPCInferenceServiceStub(
            self._channel)
        self._verbose = verbose

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()

    async def close(self):
        """
        Close the client. Any future calls to server
        will result in an Error.
        """
        await self._channel.close()

    async def infer(self,
                    infer_request: Union[InferRequest, ModelInferRequest],
                    client_timeout: Optional[float] = None,
                    headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None) \
            -> ModelInferResponse:
        """
        Run asynchronous inference using the supplied inputs.
        :param infer_request: Inference input data as InferRequest or ModelInferRequest object.
        :param client_timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                               The default value is None which means client will wait for the response from the server.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: Inference output as ModelInferResponse.
        :raises RPCError for non-OK-status response.
        """
        metadata = headers.items() if headers is not None else tuple()

        if isinstance(infer_request, InferRequest):
            infer_request = infer_request.to_grpc()
        if self._verbose:
            logger.info("metadata: {}\n infer_request: {}".format(metadata, infer_request))

        try:
            response = await self._client_stub.ModelInfer(
                request=infer_request,
                metadata=metadata,
                timeout=client_timeout)
            if self._verbose:
                logger.info(response)
            return response
        except grpc.RpcError as rpc_error:
            logger.error("Failed to infer: %s", rpc_error, exc_info=True)
            raise rpc_error

    async def is_server_ready(self, client_timeout: Optional[float] = None,
                              headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None) -> bool:
        """
        Get readiness of the inference server.
        :param client_timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                               The default value is None which means client will wait for the response from the server.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: True if server is ready, False if server is not ready.
        :raises RPCError for non-OK-status response.
        """
        try:
            response: ServerReadyResponse = await self._client_stub.ServerReady(ServerReadyRequest(),
                                                                                timeout=client_timeout,
                                                                                metadata=headers)
            if self._verbose:
                logger.info("Server ready response: %s", response)
            return response.ready
        except grpc.RpcError as rpc_error:
            logger.error("Failed to get server readiness: %s", rpc_error, exc_info=True)
            raise rpc_error

    async def is_server_live(self, client_timeout: Optional[float] = None,
                             headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None) \
            -> bool:
        """
        Get liveness of the inference server.
        :param client_timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                               The default value is None which means client will wait for the response from the server.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: True if server is live, False if server is not live.
        :raises RPCError for non-OK-status response.
        """
        try:
            response: ServerLiveResponse = await self._client_stub.ServerLive(ServerLiveRequest(),
                                                                              timeout=client_timeout, metadata=headers)
            if self._verbose:
                logger.info("Server live response: %s", response)
            return response.live
        except grpc.RpcError as rpc_error:
            logger.error("Failed to get server liveness: %s", rpc_error, exc_info=True)
            raise rpc_error

    async def is_model_ready(self, model_name: str, client_timeout: Optional[float] = None,
                             headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None) -> bool:
        """
        Get readiness of the specified model.
        :param model_name:  The name of the model to check for readiness.
        :param client_timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                               The default value is None which means client will wait for the response from the server.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: True if model is ready, False if model is not ready.
        :raises RPCError for non-OK-status response or specified model not found.
        """
        try:
            response: ModelReadyResponse = await self._client_stub.ModelReady(ModelReadyRequest(name=model_name),
                                                                              timeout=client_timeout, metadata=headers)
            if self._verbose:
                logger.info("Model %s ready response: %s", model_name, response)
            return response.ready
        except grpc.RpcError as rpc_error:
            logger.error("Failed to get readiness of the model with name %s: %s", model_name, rpc_error, exc_info=True)
            raise rpc_error


class RESTConfig:
    """
    Configuration for REST inference client.

    :param transport (optional) An asynchronous transport class to use for sending requests over the network.
    :param retries (optional) An integer value indicating the number of retries in case of ConnectError or
                   ConnectTimeout.
    :param timeout (optional) The maximum end-to-end time, in seconds, the request is allowed to take. By default,
                   client waits for the response.
    :param http2 (optional) A boolean indicating if HTTP/2 support should be enabled. Defaults to False.
    :param cert (optional) An SSL certificate used by the requested host to authenticate the client.
                Either a path to an SSL certificate file, or two-tuple of (certificate file, key file), or
                a three-tuple of (certificate file, key file, password).
    :param verify (optional) SSL certificates (a.k.a CA bundle) used to verify the identity of requested hosts.
                  Either True (default CA bundle), a path to an SSL certificate file, an ssl.SSLContext, or False
                  (which will disable verification).
    :param auth (optional) An authentication class to use when sending inference requests. Refer httpx
    :param verbose (optional) A boolean to enable verbose logging. Defaults to False.
    """

    def __init__(self, transport: AsyncBaseTransport = None, retries: int = 3, http2: bool = False,
                 timeout: Union[float, None, tuple, httpx.Timeout] = None, cert=None,
                 verify: Union[str, bool, ssl.SSLContext] = True, auth=None, verbose: bool = False):
        self.transport = transport
        self.retries = retries
        self.http2 = http2
        self.timeout = timeout
        self.cert = cert
        self.verify = verify
        self.retries = retries
        self.auth = auth
        self.transport = transport
        self.verbose = verbose
        if self.transport is None:
            httpx.AsyncHTTPTransport(retries=self.retries, http2=self.http2, cert=self.cert, verify=self.verify)


class InferenceRESTClient:
    """
        Asynchronous REST inference client. This feature is currently in alpha and may be subject to change.
        :param config (optional) A RESTConfig object which contains client configurations.
    """

    def __init__(self, config: RESTConfig = RESTConfig()):
        self._config = config
        self._client = httpx.AsyncClient(transport=config.transport, http2=config.http2, timeout=config.timeout,
                                         auth=config.auth, verify=config.verify)

    async def predict(self, url: Union[Url, str], data: Union[PredictRequest, dict],
                      headers: Optional[Mapping[str, str]] = None,
                      timeout: Union[float, None, tuple, httpx.Timeout] = None) -> PredictResponse:
        """
        Run asynchronous inference using the supplied data. This method follows the V1 protocol specification.
        :param url: Inference url
        :param data: Input data as PredictRequest object.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. By default, client waits for the response.
        :return: Inference result as PredictResponse object.
        :raises HTTPStatusError for response codes other than 2xx.
        """
        if isinstance(data, PredictRequest):
            data = data.dict()
        if self._config.verbose:
            logger.info("url: %s", url)
            logger.info("request data: %s", data)
        res = await self._client.post(url, json=data, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info("response code: %s, content: %s", res.status_code, res.text)
        res.raise_for_status()
        return PredictResponse.parse_obj(res.json())

    async def infer(self, url: Union[Url, str], data: Union[InferRequest, dict],
                    headers: Optional[Mapping[str, str]] = None,
                    timeout: Union[float, None, tuple, httpx.Timeout] = None) -> InferResponse:
        """
        Run asynchronous inference using the supplied data. This method follows the open inference protocol(V2).
        For more info on open inference protocol visit https://github.com/kserve/open-inference-protocol.
        :param url: Inference url of the inference server.
        :param data: Input data as InferRequest object.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. By default, client waits for the response.
        :return: Inference result as InferResponse object.
        :raises HTTPStatusError for response codes other than 2xx.
        """
        if isinstance(data, InferRequest):
            data = data.to_dict()
        if self._config.verbose:
            logger.info("url: %s", url)
            logger.info("request data: %s", data)
        res = await self._client.post(url, json=data, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info("response code: %s, content: %s", res.status_code, res.text)
        res.raise_for_status()
        output = res.json()
        return InferResponse.from_rest(output.get("model_name"), response=output)

    async def is_server_ready(self, url: Union[Url, str], headers: Optional[Mapping[str, str]] = None,
                              timeout: Union[float, None, tuple, httpx.Timeout] = None) -> bool:
        """
        Get readiness of the inference server.
        :param url: Readiness url of the inference server.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. By default, client waits for the response.
        :return: True if server is ready, False if server is not ready.
        :raises HTTPStatusError for response codes other than 2xx.
        """
        res = await self._client.get(url, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info("url: %s", url)
            logger.info("response code: %s, content: %s", res.status_code, res.text)
        res.raise_for_status()
        return res.json().get("ready")

    async def is_server_live(self, url: Union[Url, str], protocol_version: Union[str, PredictorProtocol],
                             headers: Optional[Mapping[str, str]] = None,
                             timeout: Union[float, None, tuple, httpx.Timeout] = None) -> bool:
        """
        Get liveness of the inference server.
        :param url: Readiness url of the inference server.
        :param protocol_version: Inference server protocol version as string or PredictorProtocol object.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. By default, client waits for the response.
        :return: True if server is ready, False if server is not ready.
        :raises HTTPStatusError for response codes other than 2xx.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        res = await self._client.get(url, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info("url: %s, protocol_version: %s", url, protocol_version)
            logger.info("response code: %s, content: %s", res.status_code, res.text)
        res.raise_for_status()
        if (protocol_version == PredictorProtocol.REST_V1 or isinstance(protocol_version, str) and
                protocol_version.lower() == PredictorProtocol.REST_V1.value.lower()):
            is_live = res.json().get("status").lower() == "alive"
        elif (protocol_version == PredictorProtocol.REST_V2 or isinstance(protocol_version, str) and
                protocol_version.lower() == PredictorProtocol.REST_V2.value.lower()):
            is_live = res.json().get("live")
        else:
            raise UnsupportedProtocol(protocol_version=protocol_version)
        return is_live

    async def is_model_ready(self, url: Union[Url, str], protocol_version: Union[str, PredictorProtocol],
                             headers: Optional[Mapping[str, str]] = None,
                             timeout: Union[float, None, tuple, httpx.Timeout] = None) -> bool:
        """
        Get readiness of the specified model.
        :param url: Readiness url of the inference model.
        :param protocol_version: Inference server protocol version as string or PredictorProtocol object.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. By default, client waits for the response.
        :return: True if server is ready, False if server is not ready.
        :raises HTTPStatusError for response codes other than 2xx.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        # TODO: Server responds with 503 service unavailable error if model is not ready. How should we handle this
        #  in inference client ?
        res = await self._client.get(url, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info("url: %s, protocol_version: %s", url, protocol_version)
            logger.info("response code: %s, content: %s", res.status_code, res.text)
        res.raise_for_status()
        if (protocol_version == PredictorProtocol.REST_V1 or isinstance(protocol_version, str) and
                protocol_version.lower() == PredictorProtocol.REST_V1.value.lower()):
            is_ready = res.json().get("ready").lower() == "true"
        elif (protocol_version == PredictorProtocol.REST_V2 or isinstance(protocol_version, str) and
                protocol_version.lower() == PredictorProtocol.REST_V2.value.lower()):
            is_ready = res.json().get("ready")
        else:
            raise UnsupportedProtocol(protocol_version=protocol_version)
        return is_ready

    async def close(self):
        """
        Close the client, transport and proxies.
        """
        await self._client.aclose()

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()
