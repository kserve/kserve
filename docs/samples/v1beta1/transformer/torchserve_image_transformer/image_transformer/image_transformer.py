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

import io
import base64
import json
import tornado
from typing import List, Dict
from PIL import Image
import torchvision.transforms as transforms
import logging
import kserve

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)


EXPLAINER_URL_FORMAT = "http://{0}/v1/models/{1}:explain"

image_processing = transforms.Compose([
        transforms.ToTensor(),
        transforms.Normalize((0.1307,), (0.3081,))
    ])


def image_transform(instance):
    """converts the input image of Bytes Array into Tensor

    Args:
        instance (dict): The request input to make an inference
        request for.

    Returns:
        list: Returns the data key's value and converts that into a list
        after converting it into a tensor
    """
    byte_array = base64.b64decode(instance["data"])
    image = Image.open(io.BytesIO(byte_array))
    instance["data"] = image_processing(image).tolist()
    logging.info(instance)
    return instance


class ImageTransformer(kserve.Model):
    """ A class object for the data handling activities of Image Classification
    Task and returns a KServe compatible response.

    Args:
        kserve (class object): The Model class from the KServe
        module is passed here.
    """
    def __init__(self, name: str, predictor_host: str):
        """Initialize the model name, predictor host and the explainer host

        Args:
            name (str): Name of the model.
            predictor_host (str): The host in which the predictor runs.
        """
        super().__init__(name)
        self.predictor_host = predictor_host
        self.explainer_host = predictor_host
        logging.info("MODEL NAME %s", name)
        logging.info("PREDICTOR URL %s", self.predictor_host)
        logging.info("EXPLAINER URL %s", self.explainer_host)
        self.timeout = 100

    def preprocess(self, inputs: Dict) -> Dict:
        """Pre-process activity of the Image Input data.

        Args:
            inputs (Dict): KServe http request

        Returns:
            Dict: Returns the request input after converting it into a tensor
        """
        return {'instances': [image_transform(instance) for instance in inputs['instances']]}

    def postprocess(self, inputs: List) -> List:
        """Post process function of Torchserve on the KServe side is
        written here.

        Args:
            inputs (List): The list of the inputs

        Returns:
            List: If a post process functionality is specified, it converts that into
            a list.
        """
        return inputs

    async def explain(self, request: Dict) -> Dict:
        """Returns the captum explanations for the input request

        Args:
            request (Dict): http input request

        Raises:
            NotImplementedError: If the explainer host is not specified.
            tornado.web.HTTPError: if the response code is not 200.

        Returns:
            Dict: Returns a dictionary response of the captum explain
        """
        if self.explainer_host is None:
            raise NotImplementedError
        logging.info("Inside Image Transformer explain %s", EXPLAINER_URL_FORMAT.format(self.explainer_host, self.name))
        response = await self._http_client.fetch(
            EXPLAINER_URL_FORMAT.format(self.explainer_host, self.name),
            method='POST',
            request_timeout=self.timeout,
            body=json.dumps(request)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)
