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

import json
import ssl
from typing import Union, List, Tuple, Any, Optional, Sequence, Mapping, Dict

import grpc
import httpx
from orjson import orjson

from .constants.constants import PredictorProtocol, INFERENCE_CONTENT_LENGTH_HEADER
from .errors import UnsupportedProtocol, InvalidInput
from .logging import trace_logger as logger
from .protocol.grpc.grpc_predict_v2_pb2 import (
    ServerReadyResponse,
    ServerLiveResponse,
    ModelReadyResponse,
    ServerReadyRequest,
    ServerLiveRequest,
    ModelReadyRequest,
)
from .protocol.grpc.grpc_predict_v2_pb2_grpc import GRPCInferenceServiceStub
from .protocol.infer_type import InferRequest, InferResponse
from .utils.utils import is_v2, is_v1


class _UseClientDefault:
    """
    For `timeout=...` parameter we need to be able to indicate the default "unset" state, in a way that is distinctly
    different to using `None`.

    The default "unset" state indicates that whatever default is set on the
    client should be used. This is different to setting `None`, which
    explicitly disables the parameter, possibly overriding a client default.

    For example we use `timeout=USE_CLIENT_DEFAULT` in the `infer()` signature.
    Omitting the `timeout` parameter will send a request using whatever default
    timeout has been configured on the client. Including `timeout=None` will
    ensure no timeout is used.

    Note that user code shouldn't need to use the `USE_CLIENT_DEFAULT` constant,
    but it is used internally when a parameter is not included.
    """


USE_CLIENT_DEFAULT = _UseClientDefault()


class InferenceGRPCClient:
    """
    Asynchronous GRPC inference client. This feature is currently in alpha and may be subject to change.
    Note: This client uses a default retry config. To override, explicitly provide the 'method_config' in channel
    options or to disable retry set the channel option ("grpc.enable_retries", 0).
    {
        "methodConfig": [
            {
                # Apply retry to all methods
                "name": [{}],
                "retryPolicy": {
                    "maxAttempts": 3,
                    "initialBackoff": "0.1s",
                    "maxBackoff": "1s",
                    "backoffMultiplier": 2,
                    "retryableStatusCodes": ["UNAVAILABLE"],
                },
            }
        ]
    }
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
    :param timeout (optional) The maximum end-to-end time, in seconds, the request is allowed to take. By default,
                   client timeout is 60 seconds. To disable timeout explicitly set it to 'None'.
    :param retries (optional) The number of retries if the request fails. This will be ignored if retry policy is provided in the 'channel_args'.
    """

    def __init__(
        self,
        url: str,
        verbose: bool = False,
        use_ssl: bool = False,
        root_certificates: str = None,
        private_key: str = None,
        certificate_chain: str = None,
        creds: grpc.ChannelCredentials = None,
        channel_args: List[Tuple[str, Any]] = None,
        timeout: Optional[float] = 60,
        retries: Optional[int] = 3,
    ):

        # requires appending the port to the predictor host for gRPC to work
        if ":" not in url:
            port = 443 if use_ssl else 80
            url = f"{url}:{port}"
        # Default retry config
        service_config_json = json.dumps(
            {
                "methodConfig": [
                    {
                        # Apply retry to all methods
                        "name": [{}],
                        "retryPolicy": {
                            "maxAttempts": retries,
                            "initialBackoff": "0.1s",
                            "maxBackoff": "1s",
                            "backoffMultiplier": 2,
                            "retryableStatusCodes": ["UNAVAILABLE"],
                        },
                    }
                ]
            }
        )
        # Explicitly check "is not None" here to support passing an empty
        # list to specify setting no channel arguments.
        if channel_args is not None:
            channel_opt = channel_args
            if ("grpc.enable_retries", 0) not in channel_opt:
                is_exist = False
                for key, _ in channel_opt:
                    if key == "grpc.service_config":
                        is_exist = True
                        break
                if ("grpc.enable_retries", 1) not in channel_opt:
                    channel_opt.append(("grpc.enable_retries", 1))
                if not is_exist and retries > 0:
                    channel_opt.append(("grpc.service_config", service_config_json))
        else:
            # To specify custom channel_opt, see the channel_args parameter.
            channel_opt = [
                ("grpc.max_send_message_length", -1),
                ("grpc.max_receive_message_length", -1),
            ]
            if retries > 0:
                channel_opt.append(("grpc.enable_retries", 1))
                channel_opt.append(("grpc.service_config", service_config_json))

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
            creds = grpc.ssl_channel_credentials(
                root_certificates=rc_bytes,
                private_key=pk_bytes,
                certificate_chain=cc_bytes,
            )
            self._channel = grpc.aio.secure_channel(url, creds, options=channel_opt)
        else:
            self._channel = grpc.aio.insecure_channel(url, options=channel_opt)
        self._client_stub = GRPCInferenceServiceStub(self._channel)
        self._verbose = verbose
        self._timeout = timeout

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

    async def infer(
        self,
        infer_request: InferRequest,
        timeout: Union[Optional[float], _UseClientDefault] = USE_CLIENT_DEFAULT,
        headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None,
    ) -> InferResponse:
        """
        Run asynchronous inference using the supplied inputs.
        :param infer_request: Inference input data as InferRequest or ModelInferRequest object.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                        The default value is 60 seconds. To disable timeout explicitly set it to 'None'.
                        This will override the client's timeout.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: Inference output as ModelInferResponse.
        :raises RPCError for non-OK-status response.
        """
        metadata = headers if headers is not None else tuple()

        if isinstance(infer_request, InferRequest):
            infer_request = infer_request.to_grpc()
        else:
            raise InvalidInput("Invalid input format")
        if self._verbose:
            logger.info(
                "metadata: {}\n infer_request: {}".format(metadata, infer_request)
            )

        try:
            response = await self._client_stub.ModelInfer(
                request=infer_request,
                metadata=metadata,
                timeout=(
                    self._timeout if isinstance(timeout, _UseClientDefault) else timeout
                ),
            )
            response = InferResponse.from_grpc(response)
            if self._verbose:
                logger.info("infer response: %s", response)
            return response
        except grpc.RpcError as rpc_error:
            logger.error("Failed to infer: %s", rpc_error, exc_info=True)
            raise rpc_error

    async def is_server_ready(
        self,
        timeout: Union[Optional[float], _UseClientDefault] = USE_CLIENT_DEFAULT,
        headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None,
    ) -> bool:
        """
        Get readiness of the inference server.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                        The default value is 60 seconds. To disable timeout explicitly set it to 'None'.
                        This will override the client's timeout.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: True if server is ready, False if server is not ready.
        :raises RPCError for non-OK-status response.
        """
        try:
            response: ServerReadyResponse = await self._client_stub.ServerReady(
                ServerReadyRequest(),
                timeout=(
                    self._timeout if isinstance(timeout, _UseClientDefault) else timeout
                ),
                metadata=headers,
            )
            if self._verbose:
                logger.info("Server ready response: %s", response)
            return response.ready
        except grpc.RpcError as rpc_error:
            logger.error("Failed to get server readiness: %s", rpc_error, exc_info=True)
            raise rpc_error

    async def is_server_live(
        self,
        timeout: Union[Optional[float], _UseClientDefault] = USE_CLIENT_DEFAULT,
        headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None,
    ) -> bool:
        """
        Get liveness of the inference server.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                        The default value is 60 seconds. To disable timeout explicitly set it to 'None'.
                        This will override the client's timeout.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: True if server is live, False if server is not live.
        :raises RPCError for non-OK-status response.
        """
        try:
            response: ServerLiveResponse = await self._client_stub.ServerLive(
                ServerLiveRequest(),
                timeout=(
                    self._timeout if isinstance(timeout, _UseClientDefault) else timeout
                ),
                metadata=headers,
            )
            if self._verbose:
                logger.info("Server live response: %s", response)
            return response.live
        except grpc.RpcError as rpc_error:
            logger.error("Failed to get server liveness: %s", rpc_error, exc_info=True)
            raise rpc_error

    async def is_model_ready(
        self,
        model_name: str,
        timeout: Union[Optional[float], _UseClientDefault] = USE_CLIENT_DEFAULT,
        headers: Union[grpc.aio.Metadata, Sequence[Tuple[str, str]], None] = None,
    ) -> bool:
        """
        Get readiness of the specified model.
        :param model_name:  The name of the model to check for readiness.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take.
                        The default value is 60 seconds. To disable timeout explicitly set it to 'None'.
                        This will override the client's timeout.
        :param headers: (optional) Additional headers to be transmitted with the request.
        :return: True if model is ready, False if model is not ready.
        :raises RPCError for non-OK-status response or specified model not found.
        """
        try:
            response: ModelReadyResponse = await self._client_stub.ModelReady(
                ModelReadyRequest(name=model_name),
                timeout=(
                    self._timeout if isinstance(timeout, _UseClientDefault) else timeout
                ),
                metadata=headers,
            )
            if self._verbose:
                logger.info("Model %s ready response: %s", model_name, response)
            return response.ready
        except grpc.RpcError as rpc_error:
            logger.error(
                "Failed to get readiness of the model with name %s: %s",
                model_name,
                rpc_error,
                exc_info=True,
            )
            raise rpc_error


class RESTConfig:
    """
    Configuration for REST inference client.

    :param transport (optional) An asynchronous transport class to use for sending requests over the network.
    :param protocol (optional) Inference server protocol as string or PredictorProtocol object. Defaults to V1 protocol.
    :param retries (optional) An integer value indicating the number of retries in case of ConnectError or
                   ConnectTimeout. Defaults to 3.
    :param timeout (optional) The maximum end-to-end time, in seconds, the request is allowed to take. By default,
                   client timeout is 60 seconds. To disable timeout explicitly set it to None.
    :param http2 (optional) A boolean indicating if HTTP/2 support should be enabled. Defaults to False.
    :param cert (optional) An SSL certificate used by the requested host to authenticate the client.
                Either a path to an SSL certificate file, or two-tuple of (certificate file, key file), or
                a three-tuple of (certificate file, key file, password).
    :param verify (optional) SSL certificates (a.k.a CA bundle) used to verify the identity of requested hosts.
                  Either True (default CA bundle), a path to an SSL certificate file, a ssl.SSLContext, or False
                  (which will disable verification).
    :param auth (optional) An authentication class to use when sending inference requests. Refer httpx
    :param verbose (optional) A boolean to enable verbose logging. Defaults to False.
    """

    def __init__(
        self,
        transport: httpx.AsyncBaseTransport = None,
        protocol: Union[str, PredictorProtocol] = "v1",
        retries: int = 3,
        http2: bool = False,
        timeout: Union[float, None, tuple, httpx.Timeout] = 60,
        cert=None,
        verify: Union[str, bool, ssl.SSLContext] = True,
        auth=None,
        verbose: bool = False,
    ):
        self.transport = transport
        self.protocol = (
            protocol.value if isinstance(protocol, PredictorProtocol) else protocol
        )
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
            httpx.AsyncHTTPTransport(
                retries=self.retries,
                http2=self.http2,
                cert=self.cert,
                verify=self.verify,
            )


class InferenceRESTClient:
    """
    Asynchronous REST inference client. This feature is currently in alpha and may be subject to change.
    :param config (optional) A RESTConfig object which contains client configurations.
    """

    def __init__(self, config: RESTConfig = None):
        self._config = RESTConfig() if config is None else config
        self._client = httpx.AsyncClient(
            transport=self._config.transport,
            http2=self._config.http2,
            timeout=self._config.timeout,
            auth=self._config.auth,
            verify=self._config.verify,
        )

    def _construct_url(
        self, base_url: Union[str, httpx.URL], relative_url: str
    ) -> httpx.URL:
        """
        Merge a relative url argument together with any 'base_url' to create the URL used for the outgoing request.
        :param base_url: The base url as str or httpx.URL object to use when constructing request url.
        :param relative_url: The relative url to use for merging with base url as string.
        :return: a httpx.URL object
        :raises InvalidURL if the base url is not valid.
        """
        if isinstance(base_url, str):
            base_url = httpx.URL(base_url)
        if base_url.scheme not in ("http", "https"):
            raise httpx.InvalidURL(
                "Base url should have 'http://' or 'https://' protocol"
            )
        if base_url.is_relative_url:
            raise httpx.InvalidURL("Base url should not be a relative url")
        if not base_url.raw_path.endswith(b"/") and not relative_url.startswith("/"):
            relative_url = "/" + relative_url
        return base_url.join(base_url.path + relative_url)

    def _construct_http_status_error(
        self, response: httpx.Response
    ) -> httpx.HTTPStatusError:
        message = (
            "{error_message}, '{0.status_code} {0.reason_phrase}' for url '{0.url}'"
        )
        error_message = ""
        if (
            "content-type" in response.headers
            and response.headers["content-type"] == "application/json"
        ):
            error_message = response.json()
            if "error" in error_message:
                error_message = error_message["error"]
        message = message.format(response, error_message=error_message)
        return httpx.HTTPStatusError(
            message, request=response.request, response=response
        )

    async def infer(
        self,
        base_url: Union[httpx.URL, str],
        data: Union[InferRequest, dict],
        model_name: Optional[str] = None,
        headers: Optional[Mapping[str, str]] = None,
        response_headers: Dict[str, str] = None,
        is_graph_endpoint: bool = False,
        timeout: Union[float, None, tuple, httpx.Timeout] = httpx.USE_CLIENT_DEFAULT,
    ) -> Union[InferResponse, Dict]:
        """
        Run asynchronous inference using the supplied data.
        :param base_url: Base url of the inference server. E.g. https://example.com:443, https://example.com:443/serving
        :param data: Input data as InferRequest object.
        :param model_name: (optional) Name of the model as string. If is_graph_endpoint is true this can be omitted.
               If is_graph_endpoint is False and model_name is None, this will raise ValueError.
        :param headers: (optional) HTTP headers to include when sending request.
        :param is_graph_endpoint: (optional) If set to True the base_url will be considered as an inference graph
                                  endpoint and will be used as it is for making the request regardless of the
                                  protocol specified in the RESTConfig. The result will be returned as dict object.
                                  Defaults to False.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. The default value is 60 seconds.
                        To disable timeout explicitly set it to 'None'.
        :return: Inference result as InferResponse object or python dict.
        :raises HTTPStatusError for response codes other than 2xx.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        if is_graph_endpoint:
            url = base_url
        elif model_name is None:
            raise ValueError("model_name should not be 'None'")
        elif is_v1(self._config.protocol):
            url = self._construct_url(
                base_url, f"{self._config.protocol}/models/{model_name}:predict"
            )
        elif is_v2(self._config.protocol):
            url = self._construct_url(
                base_url, f"{self._config.protocol}/models/{model_name}/infer"
            )
        else:
            raise UnsupportedProtocol(self._config.protocol)
        if self._config.verbose:
            logger.info("url: %s", url)
            logger.info("request data: %s", data)
        if isinstance(data, InferRequest):
            data, json_length = data.to_rest()
            if json_length:
                headers = headers or {}
                headers[INFERENCE_CONTENT_LENGTH_HEADER] = str(json_length)
                headers["content-type"] = "application/octet-stream"
        if isinstance(data, dict):
            data = orjson.dumps(data)
        response = await self._client.post(
            url, content=data, headers=headers, timeout=timeout
        )
        if self._config.verbose:
            logger.info(
                "response code: %s, content: %s", response.status_code, response.text
            )
        if not response.is_success:
            raise self._construct_http_status_error(response)
        if response_headers is not None:
            response_headers.update(response.headers)
        # If inference graph result, return it as dict
        if is_graph_endpoint:
            output = orjson.loads(response.content)
        elif is_v2(self._config.protocol):
            json_length = response.headers.get(
                INFERENCE_CONTENT_LENGTH_HEADER, len(response.content)
            )
            output = InferResponse.from_bytes(response.content, int(json_length))
        # Should be v1 protocol result, return it as dict
        else:
            output = orjson.loads(response.content)
        return output

    async def explain(
        self,
        base_url: Union[httpx.URL, str],
        model_name: str,
        data: Dict,
        headers: Optional[Mapping[str, str]] = None,
        timeout: Union[float, None, tuple, httpx.Timeout] = httpx.USE_CLIENT_DEFAULT,
    ) -> Dict:
        """
        Run asynchronous explanation using the supplied data.
        :param base_url: Base url of the inference server. E.g. https://example.com:443, https://example.com:443/serving
        :param model_name: Name of the model as string.
        :param data: Input data as python dict.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. The default value is 60 seconds.
                        To disable timeout explicitly set it to 'None'.
        :return: Explain result as python dict.
        :raises HTTPStatusError for response codes other than 2xx.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        if is_v1(self._config.protocol):
            url = self._construct_url(
                base_url, f"{self._config.protocol}/models/{model_name}:explain"
            )
        else:
            raise UnsupportedProtocol(self._config.protocol)
        if self._config.verbose:
            logger.info("url: %s", url)
            logger.info("request data: %s", data)
        data = orjson.dumps(data)
        response = await self._client.post(
            url, content=data, headers=headers, timeout=timeout
        )
        if self._config.verbose:
            logger.info(
                "response code: %s, content: %s", response.status_code, response.text
            )
        if not response.is_success:
            raise self._construct_http_status_error(response)
        return orjson.loads(response.content)

    async def is_server_ready(
        self,
        base_url: Union[httpx.URL, str],
        headers: Optional[Mapping[str, str]] = None,
        timeout: Union[float, None, tuple, httpx.Timeout] = httpx.USE_CLIENT_DEFAULT,
    ) -> bool:
        """
        Get readiness of the inference server.
        :param base_url: Base url of the inference server. E.g. https://example.com:443, https://example.com:443/serving
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. The default value is 60 seconds.
                        To disable timeout explicitly set it to 'None'.
        :return: True if server is ready, False if server is not ready.
        :raises HTTPStatusError for response codes other than 2xx.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        if is_v2(self._config.protocol):
            url = self._construct_url(base_url, f"{self._config.protocol}/health/ready")
        else:
            raise UnsupportedProtocol(protocol_version=self._config.protocol)
        if self._config.verbose:
            logger.info("url: %s, protocol_version: %s", url, self._config.protocol)
        response = await self._client.get(url, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info(
                "response code: %s, content: %s", response.status_code, response.text
            )
        if not response.is_success:
            raise self._construct_http_status_error(response)
        return response.json().get("ready")

    async def is_server_live(
        self,
        base_url: Union[str, httpx.URL],
        headers: Optional[Mapping[str, str]] = None,
        timeout: Union[float, None, tuple, httpx.Timeout] = httpx.USE_CLIENT_DEFAULT,
    ) -> bool:
        """
        Get liveness of the inference server.
        :param base_url: Base url of the inference server. E.g. https://example.com:443, https://example.com:443/serving
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. The default value is 60 seconds.
                        To disable timeout explicitly set it to 'None'.
        :return: True if server is ready, False if server is not ready.
        :raises HTTPStatusError for response codes other than 2xx.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        if is_v1(self._config.protocol):
            url = self._construct_url(base_url, "")
        elif is_v2(self._config.protocol):
            url = self._construct_url(base_url, f"{self._config.protocol}/health/live")
        else:
            raise UnsupportedProtocol(protocol_version=self._config.protocol)
        if self._config.verbose:
            logger.info("url: %s, protocol_version: %s", url, self._config.protocol)
        response = await self._client.get(url, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info(
                "response code: %s, content: %s", response.status_code, response.text
            )
        if not response.is_success:
            raise self._construct_http_status_error(response)
        if is_v1(self._config.protocol):
            is_live = response.json().get("status").lower() == "alive"
        elif is_v2(self._config.protocol):
            is_live = response.json().get("live")
        else:
            raise UnsupportedProtocol(protocol_version=self._config.protocol)
        return is_live

    async def is_model_ready(
        self,
        base_url: Union[httpx.URL, str],
        model_name: str,
        headers: Optional[Mapping[str, str]] = None,
        timeout: Union[float, None, tuple, httpx.Timeout] = httpx.USE_CLIENT_DEFAULT,
    ) -> bool:
        """
        Get readiness of the specified model.
        :param base_url: Base url of the inference server. E.g. https://example.com:443, https://example.com:443/serving
        :param model_name: Name of the model as string.
        :param headers: (optional) HTTP headers to include when sending request.
        :param timeout: (optional) The maximum end-to-end time, in seconds, the request is allowed to take. This will
                        override the timeout in the RESTConfig. The default value is 60 seconds.
                        To disable timeout explicitly set it to 'None'.
        :return: True if server is ready, False if server is not ready.
        :raises HTTPStatusError for response codes other than 2xx for v1 protocol.
        :raises UnsupportedProtocol if the specified protocol version is not supported.
        """
        if is_v1(self._config.protocol):
            url = self._construct_url(
                base_url, f"{self._config.protocol}/models/{model_name}"
            )
        elif is_v2(self._config.protocol):
            url = self._construct_url(
                base_url, f"{self._config.protocol}/models/{model_name}/ready"
            )
        else:
            raise UnsupportedProtocol(protocol_version=self._config.protocol)
        if self._config.verbose:
            logger.info("url: %s, protocol_version: %s", url, self._config.protocol)
        response = await self._client.get(url, headers=headers, timeout=timeout)
        if self._config.verbose:
            logger.info(
                "response code: %s, content: %s", response.status_code, response.text
            )

        if is_v1(self._config.protocol):
            # According to V1 protocol, the response should be a json object with ready: true/false
            # but KServe returns ready: true when ready, and 503 when not ready
            if response.status_code == httpx.codes.SERVICE_UNAVAILABLE:
                return False
            # Raise for other status codes
            if not response.is_success:
                raise self._construct_http_status_error(response)
            return response.json().get("ready")
        if is_v2(self._config.protocol):
            # According to V2, 200 status code indicates true and a 4xx status code indicates false.
            # The HTTP response body should be empty.
            # However, KServe returns 503 when not ready
            return response.is_success
        # Should not reach here, this exception should be raised in the beginning of this function
        raise UnsupportedProtocol(protocol_version=self._config.protocol)

    async def close(self):
        """
        Close the client, transport and proxies.
        """
        await self._client.aclose()

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()
