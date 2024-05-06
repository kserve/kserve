import sklearn
import shap
import dill
import joblib

# Load data
X,y = shap.datasets.adult()

# Filter columns, removing sensitive features
sensitive_features = ["Sex", "Race"]
X = X.drop(columns=sensitive_features)

X_train, X_valid, y_train, y_valid = sklearn.model_selection.train_test_split(X, y, test_size=0.2, random_state=7)

# Train knn
knn = sklearn.neighbors.KNeighborsClassifier()
knn.fit(X_train, y_train)

# Fit Shap Kernel Explainer
f = lambda x: knn.predict_proba(x)[:,1]
med = X_train.median().values.reshape((1,X_train.shape[1]))
explainer = shap.KernelExplainer(f, med)

# Export model and explainer artefacts
joblib.dump(value=knn, filename='./model.joblib')
dill.dump(obj=explainer, file='./explainer.dill')
