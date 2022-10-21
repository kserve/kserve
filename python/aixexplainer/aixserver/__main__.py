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

import kserve
from .model import AIXModel

DEFAULT_MODEL_NAME = "aixserver"
DEFAULT_EXPLAINER_TYPE = "LimeImages"
DEFAULT_NUM_SAMPLES = "1000"
DEFAULT_SEGMENTATION_ALGORITHM = "quickshift"
DEFAULT_TOP_LABELS = "10"
DEFAULT_MIN_WEIGHT = "0.01"
DEFAULT_POSITIVE_ONLY = "true"
# The required parameter is predictor_host

parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME,
                    help='The name that the model is served under.')
parser.add_argument('--num_samples', default=DEFAULT_NUM_SAMPLES,
                    help='The number of samples the explainer is allowed to take.')
parser.add_argument('--segmentation_algorithm', default=DEFAULT_SEGMENTATION_ALGORITHM,
                    help='The algorithm used for segmentation.')
parser.add_argument('--top_labels', default=DEFAULT_TOP_LABELS,
                    help='The number of most likely classifications to return.')
parser.add_argument('--min_weight', default=DEFAULT_MIN_WEIGHT,
                    help='The minimum weight needed by a pixel to be considered useful as an explanation.')
parser.add_argument('--positive_only', default=DEFAULT_POSITIVE_ONLY,
                    help='Whether or not to show only the explanations that positively indicate a classification.')
parser.add_argument('--explainer_type', default=DEFAULT_EXPLAINER_TYPE,
                    help='What type of model explainer to use.')

parser.add_argument('--predictor_host', help='The host for the predictor.', required=True)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    model = AIXModel(name=args.model_name, predictor_host=args.predictor_host,
                     segm_alg=args.segmentation_algorithm, num_samples=args.num_samples,
                     top_labels=args.top_labels, min_weight=args.min_weight,
                     positive_only=args.positive_only, explainer_type=args.explainer_type)
    model.load()
    kserve.ModelServer().start([model])
