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

from paddleserver import PaddleModel

import kserve
from kserve import logging

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--model_dir", required=True, help="A local path to the model directory"
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    logging.configure_logging(args.log_config_file)
    model = PaddleModel(args.model_name, args.model_dir)
    model.load()
    kserve.ModelServer().start([model])
