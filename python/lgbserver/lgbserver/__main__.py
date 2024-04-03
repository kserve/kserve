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
import logging

from lgbserver.lightgbm_model_repository import LightGBMModelRepository
from lgbserver.model import LightGBMModel

import kserve
from kserve.errors import ModelMissingError

DEFAULT_LOCAL_MODEL_DIR = "/tmp/model"
DEFAULT_NTHREAD = 1

parser = argparse.ArgumentParser(
    parents=[kserve.model_server.parser]
)  # pylint:disable=c-extension-no-member
parser.add_argument(
    "--model_dir", required=True, help="A local path to the model directory"
)
parser.add_argument(
    "--nthread", default=DEFAULT_NTHREAD, help="Number of threads to use by LightGBM."
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":

    model = LightGBMModel(args.model_name, args.model_dir, args.nthread)
    try:
        model.load()
        # LightGBM doesn't support multi-process, so the number of http server workers should be 1.
        kserve.ModelServer(workers=1).start([model] if model.ready else [])
    except ModelMissingError:
        logging.error(
            f"fail to load model {args.model_name} from dir {args.model_dir},"
            f"trying to load from model repository."
        )
        model_repository = LightGBMModelRepository(args.model_dir, args.nthread)
        # LightGBM doesn't support multi-process, so the number of http server workers should be 1.
        kfserver = kserve.ModelServer(workers=1, registered_models=model_repository)
        kfserver.start([model] if model.ready else [])
