from http import HTTPStatus
import tornado
from typing import Dict, Any
import numpy as np

def _validate(body: Dict):
    if "instances" not in body:
        raise tornado.web.HTTPError(
        status_code=HTTPStatus.BAD_REQUEST,
        reason="Expected key \"instances\" in request body"
        )

#TODO clarify how instances are hanlded given lists are deprectaed in xgboost : https://github.com/dmlc/xgboost/pull/3970
def tensorflow_request_to_list(body: Dict) -> Any:
    _validate(body)
    return body["instances"]

def ndarray_to_tensorflow_response(outputs: np.ndarray) -> Dict:
    return {"predictions": outputs.tolist()}




