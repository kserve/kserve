from kfserving.protocol import ProtocolHandler
import tensorflow as tf
from tensorflow.core.framework.tensor_pb2 import TensorProto
from google.protobuf import json_format
import numpy as np
import json

class SeldonProtocol(ProtocolHandler):

    def _extractTensor(self, body):
        if "data" in body:
            dataDef = body["data"]
            if "tensor" in dataDef:
                arr = np.array(dataDef.get("tensor").get("values")).reshape(dataDef.get("tensor").get("shape"))
                return arr,"tensor"
            elif "ndarray" in dataDef:
                arr = np.array(dataDef.get("ndarray"))
                return arr,"ndarray"
            elif "tftensor" in dataDef:
                tfp = TensorProto()
                json_format.ParseDict(dataDef.get("tftensor"),tfp, ignore_unknown_fields=False)
                arr = tf.make_ndarray(tfp)
                return arr,"tftensor"
            else:
                arr = np.array([])
                return arr,"tensor"
        else:
            #TODO throw exception
            pass

    def _createSeldonDataDef(self,array, ty):
        datadef = {}
        if ty == "tensor":
            datadef["tensor"] = {
                "shape": array.shape,
                "values": array.ravel().tolist()
            }
        elif ty == "ndarray":
            datadef["ndarray"] = array.tolist()
        elif ty == "tftensor":
            tftensor = tf.make_tensor_proto(array)
            jStrTensor = json_format.MessageToJson(tftensor)
            jTensor = json.loads(jStrTensor)
            datadef["tftensor"] = jTensor

        return datadef

    def handleRequest(self, body, model):

        arr, ty = self._extractTensor(body)
        inputs = model.preprocess(arr)
        results = model.predict(inputs)
        outputs = model.postprocess(results)
        seldon_datadef = self._createSeldonDataDef(outputs, ty)
        return {"data":seldon_datadef}