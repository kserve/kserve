
import warnings

from sklearn.datasets import fetch_openml
from sklearn.exceptions import ConvergenceWarning
from sklearn.neural_network import MLPClassifier
import joblib

from aix360.datasets import MNISTDataset

data = MNISTDataset()
X_train_h, X_test_h = data.test_data[:8000], data.test_data[8000:]
y_train_h, y_test_h = data.test_labels[:8000], data.test_labels[8000:]

x = X_train_h
n_samples = len(x)
X_train_2 = x.reshape((n_samples, -1))

x = X_test_h
n_samples = len(x)
X_test_2 = x.reshape((n_samples, -1))

y_train_2 = [0 for x in range(0, len(y_train_h))]
y_test_2 = [0 for x in range(0, len(y_test_h))]

for label_iter in range(0, len(y_train_h)):
    y_train_2[label_iter] = y_train_h[label_iter].argmax()

for label_iter in range(0, len(y_test_h)):
    y_test_2[label_iter] = y_test_h[label_iter].argmax()

print(data.test_data.shape)

# Load data from https://www.openml.org/d/554
X, y = fetch_openml('mnist_784', version=1, return_X_y=True)
X = X / 255.

# rescale the data, use the traditional train/test split
X_train, X_test = X[:60000].extend(X_train_2), X[60000:].extend(X_test_2)
y_train, y_test = y[:60000].extend(y_train_2), y[60000:].extend(y_test_2)

mlp = MLPClassifier(hidden_layer_sizes=(500, 500, 500), max_iter=10, alpha=1e-4,
                    solver='sgd', verbose=10, random_state=1,
                    learning_rate_init=.1)

# this example won't converge because of CI's time constraints, so we catch the
# warning and are ignore it here
with warnings.catch_warnings():
    warnings.filterwarnings("ignore", category=ConvergenceWarning,
                            module="sklearn")
    mlp.fit(X_train, y_train)

print("Training set score: %f" % mlp.score(X_train, y_train))
print("Test set score: %f" % mlp.score(X_test, y_test))

joblib.dump(mlp, 'sklearnserver/sklearnserver/example_model/model.pkl')
