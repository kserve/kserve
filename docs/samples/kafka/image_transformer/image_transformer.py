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

import kserve
from typing import Dict
import logging
import boto3
import cv2

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)

session = boto3.Session()
client = session.client('s3', endpoint_url='http://minio-service:9000', aws_access_key_id='minio',
                        aws_secret_access_key='minio123')


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

    def preprocess(self, inputs: Dict) -> Dict:
        if inputs['EventName'] == 's3:ObjectCreated:Put':
            bucket = inputs['Records'][0]['s3']['bucket']['name']
            key = inputs['Records'][0]['s3']['object']['key']
            self._key = key
            client.download_file(bucket, key, '/tmp/' + key)
            request = image_transform('/tmp/' + key)
            return {"instances": [request]}
        raise Exception("unknown event")

    def postprocess(self, inputs: Dict) -> Dict:
        logging.info(inputs)
        index = inputs["predictions"][0]["classes"]
        logging.info("digit:" + str(index))
        client.upload_file('/tmp/' + self._key, 'digit-'+str(index), self._key)
        return inputs
