# Copyright 2021 The KServe Authors.
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

import asyncio
from typing import Dict

import numpy as np
from art.attacks.evasion.square_attack import SquareAttack
from art.estimators.classification import BlackBoxClassifierNeuralNetwork
import nest_asyncio

import kserve
from kserve.logging import logger
from kserve.model import PredictorConfig


nest_asyncio.apply()


class ARTModel(kserve.Model):  # pylint:disable=c-extension-no-member
    def __init__(
        self,
        name: str,
        predictor_config: PredictorConfig,
        adversary_type: str,
        nb_classes: str,
        max_iter: str,
    ):
        super().__init__(name, predictor_config)
        if str.lower(adversary_type) != "squareattack":
            raise Exception("Invalid adversary type: %s" % adversary_type)
        self.adversary_type = adversary_type
        self.nb_classes = int(nb_classes)
        self.max_iter = int(max_iter)
        self.ready = False
        self.count = 0

    def load(self) -> bool:
        self.ready = True
        return self.ready

    def _predict(self, x):
        n_samples = len(x)
        input_image = x.reshape((n_samples, -1))
        scoring_data = {"instances": input_image.tolist()}

        loop = asyncio.get_running_loop()
        resp = loop.run_until_complete(self.predict(scoring_data))
        prediction = np.array(resp["predictions"])
        return [1 if x == prediction else 0 for x in range(0, self.nb_classes)]

    async def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        image = payload["instances"][0]
        label = payload["instances"][1]
        try:
            inputs = np.array(image)
            label = np.array(label)
            logger.info("Calling explain on image of shape %s", (inputs.shape,))
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s"
                % (e, payload["instances"])
            )
        try:
            if str.lower(self.adversary_type) == "squareattack":
                classifier = BlackBoxClassifierNeuralNetwork(
                    self._predict,
                    inputs.shape,
                    self.nb_classes,
                    channels_first=False,
                    clip_values=(-np.inf, np.inf),
                )
                preds = np.argmax(classifier.predict(inputs, batch_size=1))
                attack = SquareAttack(estimator=classifier, max_iter=self.max_iter)
                x_adv = attack.generate(x=inputs, y=label)

                adv_preds = np.argmax(classifier.predict(x_adv))
                l2_error = np.linalg.norm(np.reshape(x_adv[0] - inputs, [-1]))

                return {
                    "explanations": {
                        "adversarial_example": x_adv.tolist(),
                        "L2 error": l2_error.tolist(),
                        "adversarial_prediction": adv_preds.tolist(),
                        "prediction": preds.tolist(),
                    }
                }
        except Exception as e:
            raise Exception("Failed to explain %s" % e)
