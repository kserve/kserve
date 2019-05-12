from typing import Dict, List


class RequestHandler(object):

    def __init__(self, request: Dict):
        self.request = request

    def validate(self):
        raise NotImplementedError

    def extract_request(self) -> List:
        raise NotImplementedError

    def wrap_response(self, response: List) -> Dict:
        raise NotImplementedError
