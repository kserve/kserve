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
from alibiexplainer.explainer import ExplainerMethod  # pylint:disable=no-name-in-module
import dill
import os
import json

DEFAULT_EXPLAINER_NAME = "explainer"
EXPLAINER_FILENAME = "explainer.dill"
CONFIG_ENV = "ALIBI_CONFIGURATION"

ENV_STORAGE_URI = "STORAGE_URI"

parser = argparse.ArgumentParser(parents=[kfserving.server.parser])
parser.add_argument('--explainer_name', default=DEFAULT_EXPLAINER_NAME,
                    help='The name of model explainer.')
parser.add_argument('--predict_url', help='The URL for the model predict function', required=True)
parser.add_argument('--type',
                    type=ExplainerMethod, choices=list(ExplainerMethod), default="anchor_tabular",
                    help='Explainer method', required=True)
parser.add_argument('--explainerUri', help='The URL of a pretrained explainer',
                    default=os.environ.get(ENV_STORAGE_URI))
parser.add_argument('--config', default=os.environ.get(CONFIG_ENV),
                    help='Custom configuration parameters')

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    # Pretrained Alibi explainer
    alibi_model = None
    if args.explainerUri is not None:
        alibi_model = os.path.join(kfserving.Storage.download(args.explainerUri),
                                   EXPLAINER_FILENAME)
        with open(alibi_model, 'rb') as f:
            alibi_model = dill.load(f)
    # Custom configuration
    if args.config is None:
        config = {}
    else:
        config = json.loads(args.config)

    explainer = AlibiExplainer(args.explainer_name,
                               args.predict_url,
                               args.protocol,
                               ExplainerMethod(args.type),
                               config,
                               alibi_model)
    explainer.load()
    kfserving.KFServer().start(models=[explainer])
