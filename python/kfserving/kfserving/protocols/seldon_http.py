from http import HTTPStatus
import tornado
import numpy as np
from typing import Dict, Tuple, List
from kfserving.protocols.request_handler import RequestHandler


def _extract_list(body: Dict) -> List:
    data_def = body["data"]
    if "tensor" in data_def:
        arr = np.array(data_def.get("tensor").get("values")).reshape(data_def.get("tensor").get("shape"))
        return arr.tolist()
    elif "ndarray" in data_def:
        return data_def.get("ndarray")
    # Not presently supported
    # elif "tftensor" in data_def:
    #    tfp = TensorProto()
    #    json_format.ParseDict(data_def.get("tftensor"),tfp, ignore_unknown_fields=False)
    #    arr = tf.make_ndarray(tfp)
    #    return arr.tolist()


def _create_seldon_data_def(array: np.array, ty: str):
    datadef = {}
    if ty == "tensor":
        datadef["tensor"] = {
            "shape": array.shape,
            "values": array.ravel().tolist()
        }
    elif ty == "ndarray":
        datadef["ndarray"] = array.tolist()
    # elif ty == "tftensor":
    #    tftensor = tf.make_tensor_proto(array)
    #    j_str_tensor = json_format.MessageToJson(tftensor)
    #    j_tensor = json.loads(j_str_tensor)
    #    datadef["tftensor"] = j_tensor

    return datadef


def _get_request_ty(request: Dict):
    data_def = request["data"]
    if "tensor" in data_def:
        return "tensor"
    elif "ndarray" in data_def:
        return "ndarray"
    elif "tftensor" in data_def:
        return "tftensor"


class SeldonRequestHandler(RequestHandler):

    def __init__(self, request: Dict):
        super().__init__(request)

    def validate(self):
        if not "data" in self.request:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"data\" in request body"
            )
        data = self.request["data"]
        if not ("tensor" in data or "ndarray" in data):
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
