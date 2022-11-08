from sklearn import svm
from sklearn import datasets
from joblib import dump
clf = svm.SVC(gamma='scale')
iris = datasets.load_iris()
X, y = iris.data, iris.target
clf.fit(X, y)
dump(clf, 'model.joblib')
