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

import sys
import pickle

import numpy as np

from sklearn.pipeline import Pipeline
from sklearn.ensemble import RandomForestClassifier
from skimage.color import gray2rgb  # since the code wants color images
from aix360.datasets import MNISTDataset


class PipeStep(object):
    """
    Wrapper for turning functions into pipeline transforms (no-fitting)
    """
    def __init__(self, step_func):
        self._step_func = step_func

    def fit(self, *args):
        return self

    def transform(self, X):
        return self._step_func(X)


path_to_save = sys.argv[1]

data = MNISTDataset()
X = data.test_data[1:1001]
Y = data.test_labels[1:1001]

X = np.stack([gray2rgb(iimg) for iimg in X.reshape((-1, 28, 28))], 0)
X = np.array(np.reshape(X, (1000, 2352)))

simple_rf_pipeline = Pipeline([('RF', RandomForestClassifier())])

simple_rf_pipeline.fit(X, Y)

with open(path_to_save, 'wb') as f:
    pickle.dump(simple_rf_pipeline, f)

print("File saved")
