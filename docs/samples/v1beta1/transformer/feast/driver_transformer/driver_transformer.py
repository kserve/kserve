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

from typing import List, Dict, Union
import logging

import http.client
import json
import numpy as np
from cloudevents.http import CloudEvent
from tritonclient.grpc import service_pb2 as pb
from tritonclient.grpc import InferResult

import kserve
from kserve import InferRequest, InferResponse
from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferResponse

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)


class DriverTransformer(kserve.Model):
    """ A class object for the data handling activities of driver ranking
    Task and returns a KServe compatible response.

    Args:
        kserve (class object): The Model class from the KServe
        module is passed here.
    """

    def __init__(self, name: str,
                 predictor_host: str,
                 protocol: str,
                 feast_serving_url: str,
                 entity_ids: List[str],
                 feature_refs: List[str]):
        """Initialize the model name, predictor host, Feast serving URL,
           entity IDs, and feature references

        Args:
            name (str): Name of the model.
            predictor_host (str): The host in which the predictor runs.
            protocol (str): The protocol in which the predictor runs.
            feast_serving_url (str): The Feast feature server URL, in the form
            of <host_name:port>
            entity_ids (List[str]): The entity IDs for which to retrieve
            features from the Feast feature store
            feature_refs (List[str]): The feature references for the
            features to be retrieved
        """
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.feast_serving_url = feast_serving_url
        self.entity_ids = entity_ids
        self.feature_refs = feature_refs
        self.feature_refs_key = [feature_refs[i].replace(":", "__") for i in range(len(feature_refs))]
        logging.info("Model name = %s", name)
        logging.info("Protocol = %s", protocol)
        logging.info("Predictor host = %s", predictor_host)
        logging.info("Feast serving URL = %s", feast_serving_url)
        logging.info("Entity ids = %s", entity_ids)
        logging.info("Feature refs = %s", feature_refs)

        self.timeout = 100

    def buildEntityRow(self, inputs) -> Dict:
        """Build an entity row and return it as a dict.

        Args:
            inputs (Dict): entity ids to identify unique entities

        Returns:
            Dict: Returns the entity id attributes as an entity row

        """
        entity_rows = {}
        for i in range(len(self.entity_ids)):
            entity_rows[self.entity_ids[i]] = [instance[i] for instance in inputs['instances']]

        return entity_rows

    def buildPredictRequest(self, inputs, features) -> Dict:
        """Build the predict request for all entities and return it as a dict.

        Args:
            inputs (Dict): entity ids from http request
            features (Dict): entity features extracted from the feature store

        Returns:
            Dict: Returns the entity ids with features

        """
        request_data = []
        for i in range(len(features["field_values"])):
            entity_req = [features["field_values"][i]["fields"][self.feature_refs_key[j]]
                          for j in range(len(self.feature_refs_key))]
            for j in range(len(self.entity_ids)):
                entity_req.append(features["field_values"][i]["fields"][self.entity_ids[j]])
                request_data.insert(i, entity_req)

        # The default protocol is v1
        result = {'instances': request_data}
        if self.protocol == "v2":
            result = {'inputs': [
                {
                    "name": "predict",
                    "shape": [len(features["field_values"]), len(self.feature_refs_key) + 1],
                    "datatype": "FP32",
                    "data": request_data
                }
            ]
            }
        if self.protocol == "grpc-v2":
            data = np.array(request_data, dtype=np.float32).flatten()
            tensor_contents = pb.InferTensorContents(fp32_contents=data)
            inputs = pb.ModelInferRequest().InferInputTensor(
                name="predict",
                shape=[len(features["field_values"]), len(self.feature_refs_key) + 1],
                datatype="FP32",
                contents=tensor_contents
            )

            result = pb.ModelInferRequest(model_name=self.name, inputs=[inputs])

        return result

    def preprocess(self, inputs: Union[Dict, CloudEvent, InferRequest],
                   headers: Dict[str, str] = None) -> Union[Dict, InferRequest]:
        """Pre-process activity of the driver input data.

        Args:
            inputs (Dict|CloudEvent|InferRequest): Body of the request, v2 endpoints pass InferRequest.
            headers (Dict): Request headers.

        Returns:
            Dict|InferRequest: Transformed inputs to ``predict`` handler or return InferRequest for predictor call.
        """

        headers = {"Content-type": "application/json", "Accept": "application/json"}
        params = {'features': self.feature_refs, 'entities': self.buildEntityRow(inputs),
                  'full_feature_names': True}
        json_params = json.dumps(params)

        conn = http.client.HTTPConnection(self.feast_serving_url)
        conn.request("GET", "/get-online-features/", json_params, headers)
        resp = conn.getresponse()
        logging.info("The online feature rest request status is %s", resp.status)
        features = json.loads(resp.read().decode())

        outputs = self.buildPredictRequest(inputs, features)

        logging.info("The input for model predict is %s", outputs)

        return outputs

    def postprocess(self, response: Union[Dict, InferResponse, ModelInferResponse], headers: Dict[str, str] = None) \
            -> Union[Dict, ModelInferResponse]:
        """Post process function of the driver ranking output data. Here we
        simply pass the raw rankings through. Convert gRPC response if needed.

        Args:
            response (Dict|InferResponse|ModelInferResponse): The response passed from ``predict`` handler.
            headers (Dict): Request headers.

        Returns:
            Dict: If a post process functionality is specified, it could convert
            raw rankings into a different list.
        """
        logging.info("The output from model predict is %s", response)
        if self.protocol == "grpc-v2":
            response = InferResult(response)
            return response.get_response(as_json=True)
        return response
