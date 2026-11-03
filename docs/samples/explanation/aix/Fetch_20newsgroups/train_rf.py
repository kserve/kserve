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
import sklearn

from sklearn.pipeline import Pipeline
from sklearn.ensemble import RandomForestClassifier
from sklearn import datasets


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

data = datasets.fetch_20newsgroups()

X = data.data[1:1001]
Y = data.target[1:1001]

vectorizer = sklearn.feature_extraction.text.TfidfVectorizer(lowercase=False)
X = vectorizer.fit_transform(X)

simple_rf_pipeline = Pipeline([("RF", RandomForestClassifier())])

simple_rf_pipeline.fit(X, Y)

simple_rf_pipeline = Pipeline(
    [("vectorizer", vectorizer), ("RF + fit", simple_rf_pipeline)]
)

with open(path_to_save, "wb") as f:
    pickle.dump(simple_rf_pipeline, f)

print("File saved")
