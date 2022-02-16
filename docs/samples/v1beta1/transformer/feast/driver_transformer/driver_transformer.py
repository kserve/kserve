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
import kserve
import http.client
import json
import numpy as np
import grpc
from tritonclient.grpc import service_pb2 as pb
from tritonclient.grpc import InferResult
from tritonclient.grpc import service_pb2_grpc as pb_grpc

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
            feast_serving_url (str): The Feast feature server URL, in the form
            of <host_name:port>
            entity_ids (List[str]): The entity IDs for which to retrieve
            features from the Feast feature store
            feature_refs (List[str]): The feature references for the
            features to be retrieved
        """
        super().__init__(name)
        #self.protocol = "v2"
        self.protocol = protocol
        #self.predictor_host = predictor_host+":8008" if self.protocol == "v2" else predictor_host
        if self.protocol == "grpc-v2":
            self.predictor_host = predictor_host[:predictor_host.find(':')]+":8033"
        else:
            self.predictor_host = predictor_host
        #self.predictor_host = predictor_host+":8033" if self.protocol == "grpc-v2" else self.predictor_host
        self.feast_serving_url = feast_serving_url
        self.entity_ids = entity_ids
        self.feature_refs = feature_refs
        self.feature_refs_key = [feature_refs[i].replace(":", "__") for i in range(len(feature_refs))]

        #channel = grpc.insecure_channel(self.predictor_host)
        #infer_client = pb_grpc.GRPCInferenceServiceStub(channel)
        #self._grpc_client_stub = infer_client

        logging.info("Model name = %s", name)
        logging.info("Protocol = %s", self.protocol)
        logging.info("Predictor host = %s", self.predictor_host)
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

        return {'instances': request_data}

    def preprocess(self, inputs: Dict) -> Union[Dict, pb.ModelInferRequest]:
        """Pre-process activity of the driver input data.

        Args:
            inputs (Dict): http request

        Returns:
            Dict: Returns the request input after ingesting online features
        """
        #headers = {"Content-type": "application/json", "Accept": "application/json"}
        #params = {'features': self.feature_refs, 'entities': self.buildEntityRow(inputs),
        #          'full_feature_names': True}
        #json_params = json.dumps(params)

        #conn = http.client.HTTPConnection(self.feast_serving_url)
        #conn.request("GET", "/get-online-features/", json_params, headers)
        #resp = conn.getresponse()
        #logging.info("The online feature rest request status is %s", resp.status)
        #features = json.loads(resp.read().decode())

        #outputs = self.buildPredictRequest(inputs, features)
        outputs = {'instances': [
            [0.12008131295442581, 654, 0.27695566415786743, 1001], 
            [0.6189895868301392, 207, 0.8609133958816528, 1002], 
            [0.9767879247665405, 757, 0.7153270840644836, 1003], 
            [0.3384897708892822, 391, 0.2760862708091736, 1004], 
            [0.5456925630569458, 882, 0.179151251912117, 1005]]}

        logging.info("The input for model predict v1 is %s", outputs)

        if self.protocol == "v2":
            outputs = {'inputs': [
                {
                    "name": "predict",
                    "shape": [5, 4],
                    "datatype": "FP32",
                    "data": [
                        [0.12008131295442581, 654, 0.27695566415786743, 1001],
                        [0.6189895868301392, 207, 0.8609133958816528, 1002],
                        [0.9767879247665405, 757, 0.7153270840644836, 1003],
                        [0.3384897708892822, 391, 0.2760862708091736, 1004],
                        [0.5456925630569458, 882, 0.179151251912117, 1005]
                    ]
                }
              ]
            }

            logging.info("The input for model predict v2 is %s", outputs)

        if self.protocol == "grpc-v2":
            data = np.array([0.3891487121582031, 56.0, 0.9877331256866455, 1001.0, 0.6400811076164246, 588.0, 0.5501800179481506, 1002.0, 0.7555902600288391, 6.0, 0.746146559715271, 1003.0, 0.8032995462417603, 719.0, 0.2690494954586029, 1004.0, 0.027227262035012245, 208.0, 0.6807799935340881, 1005.0], dtype=np.float32)
            tensor_contents = pb.InferTensorContents(fp32_contents=data)
            inputs = pb.ModelInferRequest().InferInputTensor(
                    name="predict",
                    shape=[5,4],
                    datatype="FP32",
                    contents=tensor_contents
            )
            
            outputs = pb.ModelInferRequest(model_name=self.name, inputs=[inputs])

            #outputs = {'inputs': [
            #    {
            #        "name": "predict", 
            #        "shape": [5, 4], 
            #        "datatype": "FP32", 
            #        "contents": { "fp32_contents": 
            #            [0.3891487121582031, 56.0, 0.9877331256866455, 1001.0, 0.6400811076164246, 588.0, 0.5501800179481506, 1002.0, 0.7555902600288391, 6.0, 0.746146559715271, 1003.0, 0.8032995462417603, 719.0, 0.2690494954586029, 1004.0, 0.027227262035012245, 208.0, 0.6807799935340881, 1005.0]
            #        }
            #    }
            #  ]
            #}
            logging.info("The input for model predict grpc-v2 is %s", outputs)
        
        return outputs

    def postprocess(self, inputs: Union[Dict, pb.ModelInferResponse]) -> List:
        """Post process function of the driver ranking output data. Here we
        simply pass the raw rankings through.

        Args:
            inputs (List): The list of the inputs

        Returns:
            List: If a post process functionality is specified, it could convert
            raw rankings into a different list.
        """
        logging.info("The output from model predict is %s", inputs)
        if isinstance(inputs, pb.ModelInferResponse):
            response = InferResult(inputs)
            return response.get_response(as_json=True)

        return inputs
