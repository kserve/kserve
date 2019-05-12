from http import HTTPStatus
import tornado
from typing import Dict, List
from kfserving.protocols.request_handler import RequestHandler


class TensorflowRequestHandler(RequestHandler):

    def __init__(self, request: Dict):
        super().__init__(request)

    def validate(self):
        if "instances" not in self.request:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"instances\" in request body"
            )

    def extract_request(self) -> List:
        return self.request["instances"]

    def wrap_response(self, response: List) -> Dict:
        return {"predictions": response}
