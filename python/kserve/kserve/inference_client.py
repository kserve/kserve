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

import grpc

from .protocol.infer_type import InferRequest
from .protocol.grpc.grpc_predict_v2_pb2_grpc import GRPCInferenceServiceStub
from .logging import logger


class InferenceServerClient:
    def __init__(
            self,
            url,
            verbose=False,
            ssl=False,
            root_certificates=None,
            private_key=None,
            certificate_chain=None,
            creds=None,
            channel_args=None,
    ):

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
            self._channel = grpc.secure_channel(url, creds, options=channel_opt)
        elif ssl:
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
            self._channel = grpc.secure_channel(url, creds, options=channel_opt)
        else:
            self._channel = grpc.insecure_channel(url, options=channel_opt)
        self._client_stub = GRPCInferenceServiceStub(self._channel)
        self._verbose = verbose

    def __enter__(self):
        return self

    def __exit__(self, type, value, traceback):
        self.close()

    def __del__(self):
        self.close()

    def close(self):
        """Close the client. Any future calls to server
        will result in an Error.
        """
        self._channel.close()

    def infer(self, infer_request: InferRequest, client_timeout=None, headers=None):
        metadata = headers.items() if headers is not None else tuple()

        request = infer_request.to_grpc()
        if self._verbose:
            logger.info("infer, metadata {}\n{}".format(metadata, request))

        try:
            response = self._client_stub.ModelInfer(
                request=request, metadata=metadata, timeout=client_timeout
            )
            if self._verbose:
                logger.info(response)
            return response
        except grpc.RpcError as rpc_error:
            raise rpc_error
