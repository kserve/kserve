import argparse

import kserve
from typing import Dict, Union

from sklearn.preprocessing import StandardScaler
from sklearn.linear_model import LogisticRegression
from aif360.algorithms.preprocessing.optim_preproc_helpers.data_preproc_functions import (
    load_preproc_data_german,
)

from kserve import InferRequest, InferResponse, logging
from kserve.protocol.grpc.grpc_predict_v2_pb2 import (
    ModelInferRequest,
    ModelInferResponse,
)


class KServeSampleModel(kserve.Model):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        dataset_orig = load_preproc_data_german(["age"])
        scale_orig = StandardScaler()
        X_train = scale_orig.fit_transform(dataset_orig.features)
        y_train = dataset_orig.labels.ravel()

        lmod = LogisticRegression()
        lmod.fit(X_train, y_train, sample_weight=dataset_orig.instance_weights)

        self.model = lmod
        self.ready = True

    def predict(
        self,
        payload: Union[Dict, InferRequest, ModelInferRequest],
        headers: Dict[str, str] = None,
    ) -> Union[Dict, InferResponse, ModelInferResponse]:
        inputs = payload["instances"]

        scale_input = StandardScaler()
        scaled_input = scale_input.fit_transform(inputs)

        predictions = self.model.predict(scaled_input)

        return {"predictions": predictions.tolist()}


parser = argparse.ArgumentParser(parents=[kserve.model_server.parser])
parser.add_argument(
    "--model_name",
    default="german-credit",
    help="The name that the model is served under.",
)
args, _ = parser.parse_known_args()

if __name__ == "__main__":
    if args.configure_logging:
        logging.configure_logging(args.log_config_file)
    model = KServeSampleModel(args.model_name)
    model.load()
    kserve.ModelServer(workers=1).start([model])
