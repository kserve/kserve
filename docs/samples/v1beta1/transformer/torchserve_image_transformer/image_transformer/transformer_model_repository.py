import logging


from kserve.model_repository import ModelRepository
import kserve


logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)


class TransformerModelRepository(ModelRepository):
    """The class object for the Image Transformer.

    Args:
        ModelRepository (class): Model Repository class object of
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
