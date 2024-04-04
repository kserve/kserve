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

import logging
from typing import Dict, Union

import boto3
import cv2

import kserve
from kserve import InferRequest, InferResponse
from kserve.protocol.grpc.grpc_predict_v2_pb2 import ModelInferResponse

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)

session = boto3.Session()
client = session.client(
    "s3",
    endpoint_url="http://minio-service:9000",
    aws_access_key_id="minio",
    aws_secret_access_key="minio123",
)
digits_bucket = "digits"


def image_transform(image):
    img = cv2.imread(image, cv2.IMREAD_GRAYSCALE)
    g = cv2.resize(255 - img, (28, 28))
    g = g.flatten() / 255.0
    return g.tolist()


class ImageTransformer(kserve.Model):
    def __init__(self, name: str, predictor_host: str):
        super().__init__(name)
        self.predictor_host = predictor_host
        self._key = None

    async def preprocess(
        self, inputs: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferRequest]:
        logging.info("Received inputs %s", inputs)
        if inputs["EventName"] == "s3:ObjectCreated:Put":
            bucket = inputs["Records"][0]["s3"]["bucket"]["name"]
            key = inputs["Records"][0]["s3"]["object"]["key"]
            self._key = key
            client.download_file(bucket, key, "/tmp/" + key)
            request = image_transform("/tmp/" + key)
            return {"instances": [request]}
        raise Exception("unknown event")

    async def postprocess(
        self,
        response: Union[Dict, InferResponse, ModelInferResponse],
        headers: Dict[str, str] = None,
    ) -> Union[Dict, ModelInferResponse]:
        logging.info("response: %s", response)
        index = response["predictions"][0]["classes"]
        logging.info("digit:" + str(index))
        upload_path = f"digit-{index}/{self._key}"
        client.upload_file("/tmp/" + self._key, digits_bucket, upload_path)
        logging.info(f"Image {self._key} successfully uploaded to {upload_path}")
        return response
