from kfserving.kfserver import Storage
from kfserving.kfserver import KFServer
from model import XGBoostModel

import logging
import os

DEFAULT_XGB_FILE = "model.bst"

class XGBoostServer(KFServer):
    def __init__(self):
        super().__init__()

        logging.info("Copying contents of directory %s to local" % self.model_dir)
        local_dir = Storage.download(self.model_dir)
        model_file = os.path.join(local_dir, DEFAULT_XGB_FILE)
        logging.info("Successfully copied %s" % self.model_dir)

        model = XGBoostModel(self.model_name, model_file)

        # Starts up a singleton model server with the arg-specified model
        logging.info("Starting XGBoost Server for model %s" % model.name)
        self.start({model.name: model})

XGBoostServer()
