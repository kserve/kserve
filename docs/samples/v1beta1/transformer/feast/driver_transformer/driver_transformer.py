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
import kfserving

from feast import Client

logging.basicConfig(level=kfserving.constants.KFSERVING_LOGLEVEL)


class DriverTransformer(kfserving.KFModel):
    """ A class object for the data handling activities of driver ranking
    Task and returns a KFServing compatible response.

    Args:
        kfserving (class object): The KFModel class from the KFServing
        modeule is passed here.
    """
    def __init__(self, name: str,
                 predictor_host: str,
                 feast_serving_url: str,
                 entity_ids: List[str],
                 feature_refs: List[str]):
        """Initialize the model name, predictor host, Feast serving URL,
           entity IDs, and feature references

        Args:
            name (str): Name of the model.
            predictor_host (str): The host in which the predictor runs.
            feast_serving_url (str): The Feast serving URL, in the form
            of <host_name:port>
            entity_ids (List[str]): The entity IDs for which to retrieve
            features from the Feast feature store
            feature_refs (List[str]): The feature references for the
            features to be retrieved
        """
        super().__init__(name)
        self.predictor_host = predictor_host
        self.client = Client(serving_url=feast_serving_url)
        self.entity_ids = entity_ids
        self.feature_refs = feature_refs

        logging.info("Model name = %s", name)
        logging.info("Predictor host = %s", predictor_host)
        logging.info("Feast serving URL = %s", feast_serving_url)
        logging.info("Entity ids = %s", entity_ids)
        logging.info("Feature refs = %s", feature_refs)

        self.timeout = 100

    def buildEntityRow(self, instance) -> Dict:
        """Build an entity row and return it as a dict.

        Args:
            instance (list): entity id attributes to identify a unique entity

        Returns:
            Dict: Returns the entity id attributes as an entity row

        """
        entity_row = {self.entity_ids[i]: instance[i] for i in range(len(instance))}
        return entity_row

    def buildPredictRequest(self, inputs, features) -> Dict:
        """Build the predict request for all entitys and return it as a dict.

        Args:
            inputs (Dict): entity ids from KFServing http request
            features (Dict): entity features extracted from the feature store

        Returns:
            Dict: Returns the entity ids with features

        """
        request_data = []
        for i in range(len(inputs['instances'])):
            entity_req = [features[self.feature_refs[j]][i] for j in range(len(self.feature_refs))]
            for j in range(len(self.entity_ids)):
                entity_req.append(inputs['instances'][i][j])
            request_data.insert(i, entity_req)

        return {'instances': request_data}

    def preprocess(self, inputs: Dict) -> Dict:
        """Pre-process activity of the driver input data.

        Args:
            inputs (Dict): KFServing http request

        Returns:
            Dict: Returns the request input after ingesting online features
        """

        entity_rows = [self.buildEntityRow(instance) for instance in inputs['instances']]
        features = self.client.get_online_features(feature_refs=self.feature_refs, entity_rows=entity_rows).to_dict()

        outputs = self.buildPredictRequest(inputs, features)

        logging.info("The input for model predict is %s", outputs)

        return outputs

    def postprocess(self, inputs: List) -> List:
        """Post process function of the driver ranking output data. Here we
        simply pass the raw rankings through.

        Args:
            inputs (List): The list of the inputs

        Returns:
            List: If a post process functionality is specified, it could convert
            raw rankings into a different list.
        """
        logging.info("The output from model predict is %s", inputs)

        return inputs
