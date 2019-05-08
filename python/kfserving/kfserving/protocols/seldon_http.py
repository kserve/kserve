from kfserving.protocols.protocol import RequestHandler
from http import HTTPStatus
import tornado
import tensorflow as tf
from tensorflow.core.framework.tensor_pb2 import TensorProto
from google.protobuf import json_format
import numpy as np
import json
from typing import Dict, Union

class SeldonProtocol(RequestHandler):

    def _extract_tensor(self, body: Dict):
        data_def = body["data"]
        if "tensor" in data_def:
            arr = np.array(data_def.get("tensor").get("values")).reshape(data_def.get("tensor").get("shape"))
            return arr,"tensor"
        elif "ndarray" in data_def:
            arr = np.array(data_def.get("ndarray"))
            return arr,"ndarray"
        elif "tftensor" in data_def:
            tfp = TensorProto()
            json_format.ParseDict(data_def.get("tftensor"),tfp, ignore_unknown_fields=False)
            arr = tf.make_ndarray(tfp)
            return arr,"tftensor"

    def _create_seldon_data_def(self, array: np.array, ty: str):
        datadef = {}
        if ty == "tensor":
            datadef["tensor"] = {
                "shape": array.shape,
                "values": array.ravel().tolist()
            }
        elif ty == "ndarray":
            datadef["ndarray"] = array.tolist()
        elif ty == "tftensor":
            tftensor = tf.make_tensor_proto(array)
            j_str_tensor = json_format.MessageToJson(tftensor)
            j_tensor = json.loads(j_str_tensor)
            datadef["tftensor"] = j_tensor

        return datadef

    def _get_request_ty(self,request: Dict):
        data_def = request["data"]
        if "tensor" in data_def:
            return "tensor"
        elif "ndarray" in data_def:
            return "ndarray"
        elif "tftensor" in data_def:
            return "tftensor"

    def validate_request(self,body: Dict):
        if not "data" in body:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"data\" in request body"
            )
        data = body["data"]
        if not ("tensor" in data or "ndarray" in data or "tftensor" in body):
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="\"data\" key should contain either \"tensor\",\"ndarray\" or \"tftensor\""
            )

    def extract_inputs(self,body: Dict) -> Union[np.array,Dict[str,np.array]]:
        arr, ty = self._extract_tensor(body)
        return arr

    def create_response(self,request: Dict,outputs: np.array) -> Dict:
        ty = self._get_request_ty(request)
        seldon_datadef = self._create_seldon_data_def(outputs, ty)
        return {"data":seldon_datadef}

