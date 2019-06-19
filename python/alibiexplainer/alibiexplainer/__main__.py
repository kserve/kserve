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
from alibiexplainer.explainer import ExplainerMethod

parser = argparse.ArgumentParser(parents=[kfserving.server.parser])
parser.add_argument('--predict_url', help='The URL for the model predict function', required=True)
parser.add_argument('--method',
                    type=ExplainerMethod, choices=list(ExplainerMethod), default="anchor_tabular",
                    help='Explainer method')
parser.add_argument('--training_data', help='The URL for the training data')

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    explainer = AlibiExplainer(args.predict_url,
                               args.protocol,
                               ExplainerMethod(args.method),
                               training_data_url=args.training_data)
    explainer.load()
    kfserving.KFServer().start(models=[], explainer=explainer)
