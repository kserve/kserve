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
from lgbserver.lightgbm_model_repository import LightGBMModelRepository
from lgbserver.model import LightGBMModel

import kserve
from kserve.errors import ModelMissingError
from kserve.logging import logger

DEFAULT_NTHREAD = 1

parser = argparse.ArgumentParser(
    parents=[kserve.model_server.parser]
)  # pylint:disable=c-extension-no-member
parser.add_argument(
    "--model_dir", required=True, help="A local path to the model directory"
)
parser.add_argument(
    "--nthread",
    default=DEFAULT_NTHREAD,
    type=int,
    help="Number of threads to use by LightGBM.",
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = LightGBMModel(args.model_name, args.model_dir, args.nthread)
    try:
        model.load()
        # LightGBM doesn't support multi-process, so the number of http server workers should be 1.
        kserve.ModelServer(workers=1).start([model])
    except ModelMissingError:
        logger.error(
            f"failed to load model {args.model_name} from dir {args.model_dir},"
            f"trying to load from model repository."
        )
        # Case 1: Model will be loaded from model repository automatically, if present
        # Case 2: In the event that the model repository is empty, it's possible that this is a scenario for
        # multi-model serving. In such a case, models are loaded dynamically using the TrainedModel.
        # Therefore, we start the server without any preloaded models
        model_repository = LightGBMModelRepository(args.model_dir, args.nthread)
        # LightGBM doesn't support multi-process, so the number of http server workers should be 1.
        server = kserve.ModelServer(workers=1, registered_models=model_repository)
        server.start([])
