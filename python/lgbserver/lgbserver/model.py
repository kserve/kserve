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

import kserve
import lightgbm as lgb
from lightgbm import Booster
import os
from typing import Dict, List, Optional
import pandas as pd
from kserve.model import ModelMissingError, InferenceError


BOOSTER_FILE = "model.bst"


class LightGBMModel(kserve.Model):
    def __init__(self, name: str, model_dir: str, nthread: int,
                 booster: Booster = None):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.nthread = nthread
        if booster is not None:
            self._booster = booster
            self.ready = True

    def load(self) -> bool:
        model_file = os.path.join(
            kserve.Storage.download(self.model_dir), BOOSTER_FILE)
        if not os.path.exists(model_file):
            raise ModelMissingError(model_file)
        self._booster = lgb.Booster(params={"nthread": self.nthread},
                                    model_file=model_file)
        self._categorical_columns = self._load_categorical(self._booster, model_file)
        self.ready = True
        return self.ready

    def _load_categorical(
        self, booster: Booster, model_file: str
    ) -> Optional[List[str]]:
        # LightGBM does not currently force trained categorical columns to
        # categorical during predict, so pull the `categorical_feature` from the
        # saved model
        # https://github.com/microsoft/LightGBM/issues/5244
        categorical_feature = None
        with open(model_file) as f:
            for line in f:
                if line.startswith("[categorical_feature: "):
                    content = (
                        line.replace("[categorical_feature: ", "")
                        .replace("]", "")
                        .strip()
                        .split(",")
                    )
                    categorical_feature = [
                        int(value) for value in content if value != ""
                    ]

        if categorical_feature is None:
            return None

        feature_name = booster.feature_name()
        return [feature_name[i] for i in categorical_feature]

    def predict(self, request: Dict) -> Dict:
        try:
            dfs = []
            for input in request['inputs']:
                dfs.append(pd.DataFrame(input, columns=self._booster.feature_name()))
            inputs = pd.concat(dfs, axis=0)

            if self._categorical_columns:
                overlap = set(self._categorical_columns) & set(inputs.columns)
                inputs = inputs.astype({column: "category" for column in overlap})

            result = self._booster.predict(inputs)
            return {"predictions": result.tolist()}
        except Exception as e:
            raise InferenceError(str(e))
