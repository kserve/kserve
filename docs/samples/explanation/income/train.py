import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.compose import ColumnTransformer
from sklearn.impute import SimpleImputer
from sklearn.preprocessing import StandardScaler, OneHotEncoder
from alibi.datasets import adult
import joblib
from sklearn.pipeline import Pipeline

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
np.random.seed(0)
clf = RandomForestClassifier(n_estimators=50)

pipeline = Pipeline([('preprocessor', preprocessor),
                     ('clf', clf)])
pipeline.fit(X_train, Y_train)

# Dump files
joblib.dump(pipeline, 'model.joblib')
joblib.dump(X_train, "train.joblib")
joblib.dump(feature_names, "features.joblib")
joblib.dump(category_map, "category_map.joblib")
