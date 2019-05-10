from http import HTTPStatus
import tornado
import numpy as np
from typing import Dict, Tuple

def _extract_tensor(body: Dict) -> Tuple[np.array,str]:
    data_def = body["data"]
    if "tensor" in data_def:
        arr = np.array(data_def.get("tensor").get("values")).reshape(data_def.get("tensor").get("shape"))
        return arr,"tensor"
    elif "ndarray" in data_def:
        arr = np.array(data_def.get("ndarray"))
        return arr,"ndarray"
    #Not presently supported
    #elif "tftensor" in data_def:
    #    tfp = TensorProto()
    #    json_format.ParseDict(data_def.get("tftensor"),tfp, ignore_unknown_fields=False)
    #    arr = tf.make_ndarray(tfp)
    #    return arr,"tftensor"

def _create_seldon_data_def(array: np.array, ty: str):
    datadef = {}
    if ty == "tensor":
        datadef["tensor"] = {
        "shape": array.shape,
        "values": array.ravel().tolist()
        }
    elif ty == "ndarray":
        datadef["ndarray"] = array.tolist()
    #elif ty == "tftensor":
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


def _validate(body: Dict):
    if not "data" in body:
        raise tornado.web.HTTPError(
        status_code=HTTPStatus.BAD_REQUEST,
        reason="Expected key \"data\" in request body"
    )
    data = body["data"]
    if not ("tensor" in data or "ndarray" in data):
        raise tornado.web.HTTPError(
        status_code=HTTPStatus.BAD_REQUEST,
        reason="\"data\" key should contain either \"tensor\",\"ndarray\""
    )


def seldon_request_to_ndarray(body: Dict) -> np.array:
    _validate(body)
    arr, ty = _extract_tensor(body)
    return arr


def ndarray_to_seldon_response(request: Dict,outputs: np.array) -> Dict:
    ty = _get_request_ty(request)
    seldon_datadef = _create_seldon_data_def(outputs, ty)
    return {"data":seldon_datadef}

