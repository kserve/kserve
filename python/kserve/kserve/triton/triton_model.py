from typing import Union, Dict, AsyncIterator, Any

import tritonserver

from .utils import create_triton_infer_request, to_infer_response
from ..logging import logger
from ..model import (
    Model,
    InferRequest,
    InferResponse,
)
from ..protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest


class TritonModel(Model):

    def __init__(
        self,
        model_name: str,
        triton_options: tritonserver.Options = None,
    ):
        super().__init__(model_name)
        self._server: tritonserver.Server = None
        self._model: tritonserver.Model = None
        if triton_options is None:
            self._options = tritonserver.Options(
                model_repository="/mnt/models",
                server_id="triton",
                exit_on_error=True,
                strict_readiness=True,
                exit_timeout=30,
                metrics=True,
                gpu_metrics=True,
                cpu_metrics=True,
            )
        else:
            self._options = triton_options

    def load(self) -> bool:
        self._server = tritonserver.Server(
            options=self._options,
        )
        logger.info("Starting Triton model server")
        self._server.start()
        logger.info("Loading model %s", self.name)
        self._model = self._server.model(self.name)
        self.ready = True
        return self.ready

    async def predict(
        self,
        payload: Union[InferRequest, ModelInferRequest],
        headers: Dict[str, str] = None,
    ) -> Union[InferResponse, AsyncIterator[Any]]:
        if isinstance(payload, ModelInferRequest):
            req = create_triton_infer_request(
                InferRequest.from_grpc(payload), self._model
            )
        elif isinstance(payload, InferRequest):
            req = create_triton_infer_request(payload, self._model)
        else:
            raise ValueError("Invalid payload type")
        async_iterator = self._model.async_infer(inference_request=req)
        async for res in async_iterator:
            return to_infer_response(res)

    def stop(self):
        self._server.stop()
