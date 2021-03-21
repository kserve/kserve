# Copyright 2019 kubeflow.org.
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

import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.compose import ColumnTransformer
from sklearn.impute import SimpleImputer
from sklearn.preprocessing import StandardScaler, OneHotEncoder
from alibi.datasets import adult
import joblib
import dill
from sklearn.pipeline import Pipeline
import alibi

# load data
data, labels, feature_names, category_map = adult()

# define train and test set
np.random.seed(0)
data_perm = np.random.permutation(np.c_[data, labels])
data = data_perm[:, :-1]
labels = data_perm[:, -1]

idx = 30000
X_train, Y_train = data[:idx, :], labels[:idx]
X_test, Y_test = data[idx + 1:, :], labels[idx + 1:]

# feature transformation pipeline
ordinal_features = [x for x in range(len(feature_names)) if x not in list(category_map.keys())]
ordinal_transformer = Pipeline(steps=[('imputer', SimpleImputer(strategy='median')),
                                      ('scaler', StandardScaler())])

categorical_features = list(category_map.keys())
categorical_transformer = Pipeline(steps=[('imputer', SimpleImputer(strategy='median')),
                                          ('onehot', OneHotEncoder(handle_unknown='ignore'))])

preprocessor = ColumnTransformer(transformers=[('num', ordinal_transformer, ordinal_features),
                                               ('cat', categorical_transformer, categorical_features)])

# train an RF model
print("Train random forest model")
np.random.seed(0)
clf = RandomForestClassifier(n_estimators=50)
pipeline = Pipeline([('preprocessor', preprocessor),
                     ('clf', clf)])
pipeline.fit(X_train, Y_train)

print("Creating an explainer")
explainer = alibi.explainers.AnchorTabular(predict_fn=lambda x: clf.predict(preprocessor.transform(x)),
                                           feature_names=feature_names,
                                           categorical_names=category_map)
explainer.fit(X_train)
explainer.predict_fn = None  # Clear explainer predict_fn as its a lambda and will be reset when loaded


print("Saving individual files")

with open("explainer.dill", 'wb') as f:
    dill.dump(explainer, f)
joblib.dump(pipeline, 'model.joblib')
