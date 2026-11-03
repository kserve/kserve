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

from predictiveserver import PredictiveServerModel, PredictiveServerModelRepository

import kserve
from kserve import logging
from kserve.errors import ModelMissingError
from kserve.logging import logger

DEFAULT_NTHREAD = 1
DEFAULT_FRAMEWORK = "sklearn"

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--model_dir", required=True, help="A local path to the model directory"
)
parser.add_argument(
    "--framework",
    default=DEFAULT_FRAMEWORK,
    type=str,
    choices=["sklearn", "xgboost", "lightgbm"],
    help="ML framework to use: sklearn, xgboost, or lightgbm (default: sklearn)",
)
parser.add_argument(
    "--nthread",
    default=DEFAULT_NTHREAD,
    type=int,
    help="Number of threads to use by XGBoost or LightGBM (default: 1)",
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)

    # Framework selection priority:
    # 1. --framework argument (from annotation via ClusterServingRuntime args)
    # 2. Default value (sklearn)
    logger.info(f"Using framework: {args.framework}")

    model = PredictiveServerModel(
        args.model_name, args.model_dir, args.framework, args.nthread
    )
    try:
        model.load()
        # Determine worker count based on framework
        # LightGBM with multi-process workers can cause hanging due to OpenMP threading issues.
        # Force workers=1 for LightGBM (internal threading is already limited via --nthread=1 default).
        # For other frameworks, use args.workers if specified, otherwise default to 1
        workers = (
            1 if args.framework == "lightgbm" else (args.workers if args.workers else 1)
        )
        kserve.ModelServer(workers=workers).start([model])
    except ModelMissingError:
        logger.error(
            f"Failed to locate model file for model {args.model_name} under dir {args.model_dir}, "
            f"trying to load from model repository."
        )
        # Case 1: Model will be loaded from model repository automatically, if present
        # Case 2: In the event that the model repository is empty, it's possible that this is a scenario for
        # multi-model serving. In such a case, models are loaded dynamically using the TrainedModel.
        # Therefore, we start the server without any preloaded models
        model_repository = PredictiveServerModelRepository(
            args.model_dir, args.framework, args.nthread
        )
        # LightGBM with multi-process workers can cause hanging due to OpenMP threading issues.
        # Force workers=1 for LightGBM (internal threading is already limited via --nthread=1 default).
        # For other frameworks, use args.workers if specified, otherwise default to 1
        workers = (
            1 if args.framework == "lightgbm" else (args.workers if args.workers else 1)
        )
        kserve.ModelServer(workers=workers, registered_models=model_repository).start(
            []
        )
