from kserve.handlers.base import BaseHandler


class LivenessHandler(BaseHandler):  # pylint:disable=too-few-public-methods
    def get(self):
        self.write({"status": "alive"})
