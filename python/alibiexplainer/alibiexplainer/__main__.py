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

import argparse
import json
import logging
import os

import dill
import kfserving
from alibiexplainer import AlibiExplainer
from alibiexplainer.explainer import ExplainerMethod  # pylint:disable=no-name-in-module

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)

DEFAULT_EXPLAINER_NAME = "explainer"
EXPLAINER_FILENAME = "explainer.dill"
CONFIG_ENV = "ALIBI_CONFIGURATION"

ENV_STORAGE_URI = "STORAGE_URI"

parser = argparse.ArgumentParser(parents=[kfserving.server.parser])
parser.add_argument('--model_name', default=DEFAULT_EXPLAINER_NAME,
                    help='The name of model explainer.')
parser.add_argument('--predictor_host', help='The host for the predictor', required=True)
parser.add_argument('--type',
                    type=ExplainerMethod, choices=list(ExplainerMethod), default="anchor_tabular",
                    help='Explainer method', required=True)
parser.add_argument('--storage_uri', help='The URI of a pretrained explainer',
                    default=os.environ.get(ENV_STORAGE_URI))
parser.add_argument('--config', default=os.environ.get(CONFIG_ENV),
                    help='Custom configuration parameters')

args, _ = parser.parse_known_args()

if __name__ == "__main__":
    # Pretrained Alibi explainer
    alibi_model = None
    if args.storage_uri is not None:
        alibi_model = os.path.join(kfserving.Storage.download(args.storage_uri),
                                   EXPLAINER_FILENAME)
        with open(alibi_model, 'rb') as f:
            logging.info("Loading Alibi model")
            alibi_model = dill.load(f)
    # Custom configuration
    if args.config is None:
        config = {}
    else:
        config = json.loads(args.config)

    explainer = AlibiExplainer(args.model_name,
                               args.predictor_host,
                               args.protocol,
                               ExplainerMethod(args.type),
                               config,
                               alibi_model)
    explainer.load()
    kfserving.KFServer().start(models=[explainer])
