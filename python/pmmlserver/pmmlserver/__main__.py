# Copyright 2021 The KServe Authors.
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

from kserve import logging
from pmmlserver import PmmlModel

import kserve
from kserve.errors import WorkersShouldBeLessThanMaxWorkersError

DEFAULT_LOCAL_MODEL_DIR = "/tmp/model"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--model_dir", required=True, help="A local path to the model directory"
)
args, _ = parser.parse_known_args()


def validate_max_workers(actual_workers: int, max_workers: int):
    if actual_workers > max_workers:
        raise WorkersShouldBeLessThanMaxWorkersError(max_workers=1)


if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = PmmlModel(args.model_name, args.model_dir)
    model.load()
    server = kserve.ModelServer()
    # pmmlserver based on [Py4J](https://github.com/bartdag/py4j) and that doesn't support multiprocess mode.
    validate_max_workers(server.workers, 1)
    server.start([model])
