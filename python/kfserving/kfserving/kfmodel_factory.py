# Copyright 2020 kubeflow.org.
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

from typing import Optional
from kfserving import KFModel, KFModelTypes, SKLearnModel, XGBoostModel, UnsupportedModelError


class KFModelFactory:
    @staticmethod
    def create_model(model_name: str,
                     model_dir: str,
                     full_model_path: str,
                     model_type: Optional[KFModelTypes]) -> KFModel:

        if model_type == KFModelTypes.Sklearn:
            return SKLearnModel(model_name, model_dir, full_model_path)
        elif model_type == KFModelTypes.Xgboost:
            return XGBoostModel(model_name, model_dir, full_model_path)
        else:
            raise UnsupportedModelError
