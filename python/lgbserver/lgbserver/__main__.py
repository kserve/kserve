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
import kserve

from kserve.model import ModelMissingError
from lgbserver.lightgbm_model_repository import LightGBMModelRepository
from lgbserver.model import LightGBMModel

DEFAULT_MODEL_NAME = "default"
DEFAULT_LOCAL_MODEL_DIR = "/tmp/model"
DEFAULT_NTHREAD = 1

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])  # pylint:disable=c-extension-no-member
parser.add_argument('--model_dir', required=True,
                    help='A URI pointer to the model directory')
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')
parser.add_argument('--nthread', default=DEFAULT_NTHREAD,
                    help='Number of threads to use by LightGBM.')
args, _ = parser.parse_known_args()

if __name__ == "__main__":

    model = LightGBMModel(args.model_name, args.model_dir, args.nthread)
    try:
        model.load()
    except ModelMissingError:
        logging.error(f"fail to load model {args.model_name} from dir {args.model_dir},"
                      f"trying to load from model repository.")
    model_repository = LightGBMModelRepository(args.model_dir, args.nthread)
    # LightGBM doesn't support multi-process, so the number of http server workers should be 1.
    kfserver = kserve.ModelServer(workers=1, registered_models=model_repository)  # pylint:disable=c-extension-no-member
    kfserver.start([model] if model.ready else [])
