from http import HTTPStatus
import tornado
import numpy as np
from typing import Dict, Tuple, List
from kfserving.protocols.request_handler import RequestHandler
from enum import Enum

class SeldonPayload(Enum):
    TENSOR = 1
    NDARRAY =2
    TFTENSOR = 3

def _extract_list(body: Dict) -> List:
    data_def = body["data"]
    if "tensor" in data_def:
        arr = np.array(data_def.get("tensor").get("values")).reshape(data_def.get("tensor").get("shape"))
        return arr.tolist()
    elif "ndarray" in data_def:
        return data_def.get("ndarray")
    else:
        raise Exception("Could not extract seldon payload %s" % body)

def _create_seldon_data_def(array: np.array, ty: SeldonPayload):
    datadef = {}
    if ty == SeldonPayload.TENSOR:
        datadef["tensor"] = {
            "shape": array.shape,
            "values": array.ravel().tolist()
        }
    elif ty == SeldonPayload.NDARRAY:
        datadef["ndarray"] = array.tolist()
    elif ty == SeldonPayload.TFTENSOR:
        raise NotImplementedError("Seldon payload %s not supported" % ty)
    else:
        raise Exception("Unknown Seldon payload %s" % ty)
    return datadef


def _get_request_ty(request: Dict) -> SeldonPayload:
    data_def = request["data"]
    if "tensor" in data_def:
        return SeldonPayload.TENSOR
    elif "ndarray" in data_def:
        return SeldonPayload.NDARRAY
    elif "tftensor" in data_def:
        return SeldonPayload.TFTENSOR


class SeldonRequestHandler(RequestHandler):

    def __init__(self, request: Dict):
        super().__init__(request)

    def validate(self):
        if not "data" in self.request:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"data\" in request body"
            )
        ty = _get_request_ty(self.request)
        if not (ty == SeldonPayload.TENSOR or ty == SeldonPayload.NDARRAY):
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="\"data\" key should contain either \"tensor\",\"ndarray\""
            )

    def extract_request(self) -> List:
        return _extract_list(self.request)

    def wrap_response(self, response: List) -> Dict:
        arr = np.array(response)
        ty = _get_request_ty(self.request)
        seldon_datadef = _create_seldon_data_def(arr, ty)
        return {"data": seldon_datadef}
