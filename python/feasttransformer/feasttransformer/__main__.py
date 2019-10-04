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
import kfserving
from feasttransformer import FeastTransformer

PARSER = argparse.ArgumentParser(parents=[kfserving.server.parser])
PARSER.add_argument('--name', type=str,
                    required=True, help='The name of model.')
PARSER.add_argument('--predictor-host', type=str, required=True,
                    help='The host for the model predictor')
PARSER.add_argument('--feast-url', type=str, required=True,
                    help='The url of the FeastServing Service.')
# PARSER.add_argument('--data-type', type=str, required=True,
#                     help='The data response type from Feast.')
PARSER.add_argument('--entity-ids', type=str, nargs='+',
                    help='A list of entity_ids to use as keys in the feature store.', required=True)
PARSER.add_argument('--feature-ids', type=str, nargs='+',
                    help='A list of features to retrieve from the feature store', required=True)
PARSER.add_argument('--flatten-features', type=bool, default=False)
PARSER.add_argument('--omit-entities', type=bool, default=False)

ARGS, _ = PARSER.parse_known_args()

if __name__ == "__main__":
    TRANSFORMER = FeastTransformer(
        name=ARGS.name,
        predictor_host=ARGS.predictor_host,
        feast_url=ARGS.feast_url,
        # data_type=ARGS.data_type,
        entity_ids=ARGS.entity_ids,
        feature_ids=ARGS.feature_ids,
        flatten_features=ARGS.flatten_features,
        omit_entities=ARGS.omit_entities)
    kfserving.KFServer().start(models=[TRANSFORMER])
