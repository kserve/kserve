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
from alibiexplainer import AlibiExplainer
from alibiexplainer.explainer import ExplainerAlgorithm, ExplainerModelType, ExplainerModelFeatures, ModelProtocol

parser = argparse.ArgumentParser(parents=[kfserving.server.parser])
parser.add_argument('--model_url', help='The URL for the model predict function', required=True)
parser.add_argument('--algorithm',
                    type=ExplainerAlgorithm, choices=list(ExplainerAlgorithm), default="anchors",
                    help='Explainer algorithhm')
parser.add_argument('--model_type', default="classification",
                    type=ExplainerModelType, choices=list(ExplainerModelType),
                    help='The type of model to explain')
parser.add_argument('--model_features', default="tabular",
                    type=ExplainerModelFeatures, choices=list(ExplainerModelFeatures),
                    help='The type of features the model accepts')
parser.add_argument('--training_data', help='The URL for the training data pickle')
parser.add_argument('--feature_names', help='The URL for the feature names pickle')
parser.add_argument('--categorical_map', help='The URL for the categorical mapping pickle')
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    explainer = AlibiExplainer(args.model_url,
                               args.protocol,
                               ExplainerAlgorithm(args.algorithm),
                               ExplainerModelType(args.model_type),
                               ExplainerModelFeatures(args.model_features),
                               training_data_uri=args.training_data,
                               feature_names_uri=args.feature_names,
                               categorical_map_uri=args.categorical_map)
    explainer.load()
    kfserving.KFServer().start(models=[],explainer=explainer)
