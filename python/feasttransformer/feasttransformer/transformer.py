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

from typing import List, Dict
import logging
import json
import requests
from kfserving.transformer import Transformer
from kfserving.server import Protocol
import kfserving
logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class FeastTransformer(Transformer):
    def __init__(self,
                 name: str,
                 predictor_host: str,
                 feast_url: str,
                 entity_ids: List[str],
                 feature_ids: List[str],
                 flatten_features: bool,
                 omit_entities: bool):
        super().__init__(name, predictor_host=predictor_host,
                         protocol=Protocol.tensorflow_http)
        self.feast_url = feast_url
        self.entity_ids = entity_ids
        self.ids = omit_entities * entity_ids + feature_ids
        self.feature_sets = self.build_feature_sets(feature_ids)
        self.flatten_features = flatten_features

    def build_feature_sets(self, feature_ids: List):
        feature_sets = {}
        for feature_id in feature_ids:
            try:
                # Extract name, version, feature
                name, version, feature = feature_id.split(':')
                name_version = ':'.join([name, version])

                # Add to featuresets
                feature_set = feature_sets.get(name_version, {})
                feature_names = feature_set.get("feature_names", [])
                feature_names.append(feature)
                feature_sets[name_version] = {
                    "name": name,
                    "version": int(version),
                    "feature_names": feature_names
                }
            except Exception as exception:
                logging.error(exception)
                raise ValueError(
                    "Invalid feature_id, want:`name:version:feature`, got %s." % feature_id)

        return feature_sets

    def preprocess(self, inputs: Dict) -> List:
        return {'instances': [self.enrich(instance) for instance in inputs['instances']]}

    def postprocess(self, inputs: List) -> List:
        return inputs

    def enrich(self, instance):
        url = "%s/api/v1/features/online" % self.feast_url
        headers = {'Content-type': 'application/json'}
        data = json.dumps({
            "featureSets": list(self.feature_sets.values()),
            "entityRows":  [{"fields": instance}]
        })
        logging.info("Requesting Feast: POST %s %s", url, data)
        response = requests.post(url=url, headers=headers, data=data)
        response_json = response.json()
        logging.info("Retrieved %s", response_json)
        return self._flatten(response_json)

    def _flatten(self, response: List) -> List:
        if not self.flatten_features: return response
        flattened_outputs = []
        for output in response:
            flattened_output = [output[id] for id in self.ids]
            flattened_outputs.append(flattened_output)
        return flattened_outputs
