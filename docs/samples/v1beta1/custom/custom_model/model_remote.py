import kfserving
from torchvision import models, transforms
from typing import Dict
import torch
from PIL import Image
import base64
import io
from ray import serve


@serve.deployment(name="kfserving-custom-model", config={"num_replicas": 2})
class KFServingSampleModel(kfserving.KFModel):
    def __init__(self):
        self.name = "kfserving-custom-model"
        super().__init__(self.name)
        self.load()

    def load(self):
        f = open('imagenet_classes.txt')
        self.classes = [line.strip() for line in f.readlines()]

        model = models.alexnet(pretrained=True)
        model.eval()
        self.model = model

        self.ready = True

    async def predict(self, request: Dict) -> Dict:
        request = await request.body()
        inputs = request["instances"]

        # Input follows the Tensorflow V1 HTTP API for binary values
        # https://www.tensorflow.org/tfx/serving/api_rest#encoding_binary_values
        data = inputs[0]["image"]["b64"]

        raw_img_data = base64.b64decode(data)
        input_image = Image.open(io.BytesIO(raw_img_data))

        preprocess = transforms.Compose([
            transforms.Resize(256),
            transforms.CenterCrop(224),
            transforms.ToTensor(),
            transforms.Normalize(mean=[0.485, 0.456, 0.406],
                                 std=[0.229, 0.224, 0.225]),
        ])

        input_tensor = preprocess(input_image)
        input_batch = input_tensor.unsqueeze(0)

        output = self.model(input_batch)

        scores = torch.nn.functional.softmax(output, dim=1)[0]

        _, top_5 = torch.topk(output, 5)

        results = {}
        for idx in top_5[0]:
            results[self.classes[idx]] = scores[idx].item()

        return {"predictions": results}


if __name__ == "__main__":
    kfserving.KFServer(workers=1).start({"kfserving-custom-model": KFServingSampleModel})
