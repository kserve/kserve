# Copyright 2019 kubeflow.org.
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

import kfserving
import argparse
from .image_transformer_v2 import ImageTransformerV2

DEFAULT_MODEL_NAME = "model"

parser = argparse.ArgumentParser(parents=[kfserving.kfserver.parser])
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')
parser.add_argument('--predictor_host', help='The URL for the model predict function', required=True)
parser.add_argument('--protocol', help='prediction protocol')

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    transformer = ImageTransformerV2(args.model_name, predictor_host=args.predictor_host, protocol=args.protocol)
    kfserver = kfserving.KFServer()
    kfserver.start(models=[transformer])
