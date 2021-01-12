import os

import logging
import tornado.ioloop
import tornado.web
import tornado.httpserver
import tornado.log

from kfserving.kfmodel_repository import KFModelRepository
import requests
import kfserving
from .image_transformer import ImageTransformer


logging.basicConfig(level=kfserving.constants.KFSERVING_LOGLEVEL)



class TransformerModelRepository(KFModelRepository):
    """The class object for the Image Transformer.

    Args:
        KFModelRepository (class): KFModel Repository class object of
        kfserving is passed here.
    """

    def __init__(self, predictor_host: str):
        """Initialize the Transformer Model Repository class object

        Args:
            predictor_host (str): The predictor host is specified here
        """
        super().__init__()
        logging.info("ImageTSModelRepo is initialized")
        self.predictor_host = predictor_host
