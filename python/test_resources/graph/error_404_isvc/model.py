# Copyright 2023 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import argparse
from typing import Dict, Union

import kserve
from fastapi import HTTPException

from kserve import logging
from kserve.model import InferRequest, ModelInferRequest


class SampleTemplateNode(kserve.Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.load()

    def load(self):
        self.ready = True

    def predict(
        self, payload: Union[Dict, InferRequest, ModelInferRequest], headers
    ) -> Dict:
        raise HTTPException(status_code=404, detail="Intentional 404 code")


parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
args, _ = parser.parse_known_args()
if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = SampleTemplateNode(name=args.model_name)
    kserve.ModelServer(workers=1).start([model])
