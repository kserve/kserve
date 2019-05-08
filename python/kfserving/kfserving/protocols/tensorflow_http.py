from http import HTTPStatus
import tornado
from kfserving.protocols.protocol import RequestHandler
from typing import Dict, Union
import numpy as np

class TFServingProtocol(RequestHandler):

    def validate_request(self,body):
        if "instances" not in body:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"instances\" in request body"
            )

    def extract_inputs(self,body: Dict) -> Union[np.array,Dict[str,np.array]]:
        #TODO decide on how the raw inputs should be processed
        return np.array(body["instances"])

    def create_response(self,request: Dict,outputs: np.array) -> Dict:
        return {"predictions": outputs.tolist()}




