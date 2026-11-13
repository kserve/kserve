# Copyright 2026 The KServe Authors.
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
import time

from kserve import Model, ModelServer


class BlockingPredictor(Model):
    def __init__(self, offload=True):
        super().__init__("blocking", offload_sync_predict=offload)
        self.ready = True

    def predict(self, payload, headers=None):
        # Simulate a synchronous I/O operation with bounded latency.
        time.sleep(3 if payload["instances"] == ["slow"] else 0)
        return {"predictions": payload["instances"]}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--inline", action="store_true")
    args = parser.parse_args()
    ModelServer(http_port=8080, enable_grpc=False, max_asyncio_workers=2).start(
        [BlockingPredictor(offload=not args.inline)]
    )
