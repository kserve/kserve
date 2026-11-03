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

from artserver import ARTModel

import kserve
from kserve import logging

DEFAULT_ADVERSARY_TYPE = "SquareAttack"

DEFAULT_MAX_ITER = "1000"
DEFAULT_NB_CLASSES = "10"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--adversary_type",
    default=DEFAULT_ADVERSARY_TYPE,
    help="What type of adversarial tool to use.",
)
parser.add_argument(
    "--max_iter", default=DEFAULT_MAX_ITER, help="The max number of iterations to run."
)
parser.add_argument(
    "--nb_classes",
    default=DEFAULT_NB_CLASSES,
    help="The number of different classification types.",
)

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = ARTModel(
        args.model_name,
        adversary_type=args.adversary_type,
        nb_classes=args.nb_classes,
        max_iter=args.max_iter,
    )
    model.load()
    kserve.ModelServer().start([model])
