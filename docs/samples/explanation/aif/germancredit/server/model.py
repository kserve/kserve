import kserve
from typing import Dict

from sklearn.preprocessing import StandardScaler
from sklearn.linear_model import LogisticRegression
from aif360.algorithms.preprocessing.optim_preproc_helpers.data_preproc_functions import load_preproc_data_german


class KServeSampleModel(kserve.Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        dataset_orig = load_preproc_data_german(['age'])
        scale_orig = StandardScaler()
        X_train = scale_orig.fit_transform(dataset_orig.features)
        y_train = dataset_orig.labels.ravel()

        lmod = LogisticRegression()
        lmod.fit(X_train, y_train,
                 sample_weight=dataset_orig.instance_weights)

        self.model = lmod
        self.ready = True

    def predict(self, request: Dict) -> Dict:
        inputs = request["instances"]

        scale_input = StandardScaler()
        scaled_input = scale_input.fit_transform(inputs)

        predictions = self.model.predict(scaled_input)

        return {"predictions":  predictions.tolist()}


if __name__ == "__main__":
    model = KServeSampleModel("german-credit")
    model.load()
    kserve.ModelServer(workers=1).start([model])
