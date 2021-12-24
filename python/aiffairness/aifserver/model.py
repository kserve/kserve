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
from typing import Dict

import asyncio
import kserve
import numpy as np
import pandas as pd
from aif360.metrics import BinaryLabelDatasetMetric
from aif360.datasets import BinaryLabelDataset


class AIFModel(kserve.Model):
    def __init__(self, name: str, predictor_host: str, feature_names: list, label_names: list, favorable_label: float,
                 unfavorable_label: float, privileged_groups: list, unprivileged_groups: list):
        super().__init__(name)
        self.name = name
        self.predictor_host = predictor_host
        self.ready = False
        self.feature_names = feature_names
        self.label_names = label_names
        self.favorable_label = favorable_label
        self.unfavorable_label = unfavorable_label
        self.privileged_groups = privileged_groups
        self.unprivileged_groups = unprivileged_groups

    def load(self):
        self.ready = True

    def _predict(self, inputs):
        scoring_data = {'instances': inputs}

        loop = asyncio.get_running_loop()
        resp = loop.run_until_complete(self.predict(scoring_data))
        return np.array(resp["predictions"])

    def explain(self, request: Dict) -> Dict:
        inputs = request["instances"]
        predictions = np.array(request["outputs"])

        dataframe_predicted = pd.DataFrame(inputs, columns=self.feature_names)
        dataframe_predicted[self.label_names[0]] = predictions

        dataset_predicted = BinaryLabelDataset(favorable_label=self.favorable_label,
                                               unfavorable_label=self.unfavorable_label,
                                               df=dataframe_predicted,
                                               label_names=self.label_names,
                                               protected_attribute_names=['age'])

        metrics = BinaryLabelDatasetMetric(dataset_predicted,
                                           unprivileged_groups=self.unprivileged_groups,
                                           privileged_groups=self.privileged_groups)

        return {
            "predictions": predictions.tolist(),
            "metrics": {
                "base_rate": metrics.base_rate(),
                "consistency": metrics.consistency().tolist(),
                "disparate_impact": metrics.disparate_impact(),
                "num_instances": metrics.num_instances(),
                "num_negatives": metrics.num_negatives(),
                "num_positives": metrics.num_positives(),
                "statistical_parity_difference": metrics.statistical_parity_difference(),
            }
        }
