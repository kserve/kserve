from typing import Dict, Union
import numpy as np

#TODO Needs further discussion if this is appropriate for the initial protocols we support: Tensorflow Serving HTTP and Seldon HTTP
class RequestHandler(object):

    def validate_request(self,body):
        """
        validates the request body
        :param body:
        """
        raise NotImplementedError()

    def extract_inputs(self,body: Dict) -> Union[np.array,Dict[str,np.array]]:
        """
        Takes JSON and extracts data
        :param body:
        :return: Numpy array or dict of kets to numpy arrays
        """
        raise NotImplementedError()

    def create_response(self,request: Dict,outputs: np.array) -> Dict:
        """
        Create a response Dict
        :param outputs:
        :return:
        """
        raise NotImplementedError()

