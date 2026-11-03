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

from typing import Dict, Union

from kserve import Model
from kserve.protocol.infer_type import InferRequest, InferResponse

# Import from existing framework servers
from sklearnserver.model import SKLearnModel
from xgbserver.model import XGBoostModel
from lgbserver.model import LightGBMModel


class PredictiveServerModel(Model):
    """
    Unified predictive model server supporting multiple ML frameworks.

    Supported frameworks:
    - sklearn: Scikit-learn models (.joblib, .pkl, .pickle)
    - xgboost: XGBoost models (.bst, .json, .ubj)
    - lightgbm: LightGBM models (.bst)
    """

    FRAMEWORK_SKLEARN = "sklearn"
    FRAMEWORK_XGBOOST = "xgboost"
    FRAMEWORK_LIGHTGBM = "lightgbm"

    SUPPORTED_FRAMEWORKS = [FRAMEWORK_SKLEARN, FRAMEWORK_XGBOOST, FRAMEWORK_LIGHTGBM]

    def __init__(self, name: str, model_dir: str, framework: str, nthread: int = 1):
        """
        Initialize the Predictive Server model.

        Args:
            name: Model name
            model_dir: Directory containing the model file
            framework: ML framework to use (sklearn, xgboost, lightgbm)
            nthread: Number of threads for XGBoost and LightGBM (default: 1)

        Raises:
            ValueError: If framework is not supported
        """
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.framework = framework.lower()
        self.nthread = nthread
        self.ready = False

        if self.framework not in self.SUPPORTED_FRAMEWORKS:
            raise ValueError(
                f"Unsupported framework: {framework}. "
                f"Supported frameworks: {', '.join(self.SUPPORTED_FRAMEWORKS)}"
            )

        # Initialize the appropriate framework model
        self._model = self._create_framework_model()

    def _create_framework_model(self) -> Model:
        """Create the appropriate framework-specific model instance."""
        if self.framework == self.FRAMEWORK_SKLEARN:
            return SKLearnModel(self.name, self.model_dir)
        elif self.framework == self.FRAMEWORK_XGBOOST:
            return XGBoostModel(self.name, self.model_dir, self.nthread)
        elif self.framework == self.FRAMEWORK_LIGHTGBM:
            return LightGBMModel(self.name, self.model_dir, self.nthread)
        else:
            # Should never reach here due to validation in __init__
            raise ValueError(f"Unsupported framework: {self.framework}")

    def load(self) -> bool:
        """Load the model using the appropriate framework."""
        self.ready = self._model.load()
        return self.ready

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        """
        Perform inference using the loaded model.

        Args:
            payload: Input data for prediction
            headers: Optional HTTP headers

        Returns:
            Prediction results
        """
        return self._model.predict(payload, headers)
