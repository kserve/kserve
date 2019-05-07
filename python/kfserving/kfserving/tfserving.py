from http import HTTPStatus
import tornado
from kfserving.protocol import ProtocolHandler

class TFServingProtocol(ProtocolHandler):

    def handleRequest(self, body, model):
        if "instances" not in body:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"instances\" in request body"
            )

        inputs = model.preprocess(body["instances"])
        results = model.predict(inputs)
        outputs = model.postprocess(results)

        return {"predictions":outputs}

