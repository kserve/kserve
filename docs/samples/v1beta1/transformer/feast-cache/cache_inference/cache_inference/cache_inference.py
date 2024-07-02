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

import json
from typing import List, Dict, Union

import requests
import numpy as np

import kserve
from kserve import InferRequest, InferResponse, InferInput
from kserve.protocol.grpc import grpc_predict_v2_pb2 as pb
from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferResponse
from kserve.logging import logger


class CacheTransformer(kserve.Model):
    """A class object for the data handling activities of driver ranking
    Task and returns a KServe compatible response.

    Args:
        kserve (class object): The Model class from the KServe
        module is passed here.
    """

    def __init__(
        self,
        name: str,
        predictor_host: str,
        protocol: str,
        feast_serving_url: str,
        entity_id_name: str,
        feature_refs: List[str],
        force_recompute: bool = False,
    ):
        """Initialize the model name, predictor host, Feast serving URL,
           entity IDs, and feature references

        Args:
            name (str): Name of the model.
            predictor_host (str): The host in which the predictor runs.
            protocol (str): The protocol in which the predictor runs.
            feast_serving_url (str): The Feast feature server URL, in the form
            of <host_name:port>
            entity_id_name (str): The entity ID name for which to retrieve
            features from the Feast feature store
            feature_refs (List[str]): The feature references for the
            features to be retrieved
            force_recompute (bool, optional): A flag to force recompute the score, otherwise it will be retrieved from Feast
        """
        super().__init__(name)
        self.predictor_host = predictor_host
        self.protocol = protocol
        self.feast_serving_url = feast_serving_url
        self.entity_id_name = entity_id_name
        self.feature_refs = feature_refs
        self.force_recompute = force_recompute
        self.feature_refs_key = [
            feature_refs[i].replace(":", "__") for i in range(len(feature_refs))
        ]
        logger.info("Model name = %s", name)
        logger.info("Protocol = %s", protocol)
        logger.info("Predictor host = %s", predictor_host)
        logger.info("Feast serving URL = %s", feast_serving_url)
        logger.info("Entity id name = %s", entity_id_name)
        logger.info("Feature refs = %s", feature_refs)

        self.timeout = 100

    def buildEntityRow(self, inputs) -> Dict:
        """Build an entity row and return it as a dict.

        Args:
            inputs (Dict): entity ids to identify unique entities

        Returns:
            Dict: Returns the entity id attributes as an entity row

        """
        entity_rows = {}
        entity_ids = []
        for instance in inputs["instances"]:
            entity_ids += instance
        entity_rows[self.entity_id_name] = entity_ids
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
        acc_rate_index = features["metadata"]["feature_names"].index(
            "driver_hourly_stats__acc_rate"
        )
        avg_daily_trips_index = features["metadata"]["feature_names"].index(
            "driver_hourly_stats__avg_daily_trips"
        )
        conv_rate_index = features["metadata"]["feature_names"].index(
            "driver_hourly_stats__conv_rate"
        )
        entity_ids_index = features["metadata"]["feature_names"].index("driver_id")

        # input format [acc_rate, avg_daily_trips, conv_rate, driver_id]
        for i in range(len(features["results"][entity_ids_index]["values"])):
            single_entity_data = [
                features["results"][acc_rate_index]["values"][i],
                features["results"][avg_daily_trips_index]["values"][i],
                features["results"][conv_rate_index]["values"][i],
                features["results"][entity_ids_index]["values"][i],
            ]
            request_data.append(single_entity_data)

        # The default protocol is v1
        request = {"instances": request_data}

        if self.protocol == "v2":
            data = np.array(request_data, dtype=np.float32).flatten()
            tensor_contents = pb.InferTensorContents(fp32_contents=data)
            infer_inputs = [
                InferInput(
                    name="INPUT_0",
                    datatype="FP32",
                    shape=[
                        len(features["results"][entity_ids_index]),
                        len(self.feature_refs_key) + 1,
                    ],
                    data=tensor_contents,
                )
            ]
            request = InferRequest(model_name=self.name, infer_inputs=infer_inputs)

        return request

    def preprocess(
        self, inputs: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferRequest]:
        """Pre-process activity of the driver input data.

        Args:
            inputs (Dict|CloudEvent|InferRequest): Body of the request, v2 endpoints pass InferRequest.
            headers (Dict): Request headers.

        Returns:
            Dict|InferRequest: Transformed inputs to ``predict`` handler or return InferRequest for predictor call.
        """

        headers = {"Content-type": "application/json", "Accept": "application/json"}
        params = {
            "features": self.feature_refs,
            "entities": self.buildEntityRow(inputs),
            "full_feature_names": True,
        }
        request_url = (
            "{0}/get-online-features".format(self.feast_serving_url)
            if "http" in self.feast_serving_url
            else "http://{0}/get-online-features".format(self.feast_serving_url)
        )
        json_params = json.dumps(params)
        logger.info("feast request url %s", request_url)
        logger.info("feast request headers %s", headers)
        logger.info("feast request body %s", json_params)

        resp = requests.post(request_url, data=json_params, headers=headers)
        logger.info("feast response status is %s", resp.status_code)
        logger.info("feast response headers %s", resp.headers)
        features = resp.json()
        logger.info("feast response body %s", features)

        outputs = self.buildPredictRequest(inputs, features)
        logger.info("The input for model predict is %s", outputs)

        return outputs

    def postprocess(
        self,
        response: Union[Dict, InferResponse, ModelInferResponse],
        headers: Dict[str, str] = None,
    ) -> Union[Dict, ModelInferResponse]:
        """Post process function of the driver ranking output data. Here we
        simply pass the raw rankings through. Convert gRPC response if needed.

        Args:
            response (Dict|InferResponse|ModelInferResponse): The response passed from ``predict`` handler.
            headers (Dict): Request headers.

        Returns:
            Dict: If a post process functionality is specified, it could convert
            raw rankings into a different list.
        """
        logger.info("The output from model predict is %s", response)
        return response
