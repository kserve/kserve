class KFModel(object):
    def predict(self, inputs):
            raise NotImplementedError
    
    def preprocess(self, inputs):
        return inputs
        
    def postprocess(self, outputs):
        return outputs
