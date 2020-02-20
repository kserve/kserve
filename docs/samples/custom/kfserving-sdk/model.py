import kfserving
from torchvision import models, transforms
from typing import List, Dict
import torch
from PIL import Image
import base64
import io


class KFServingSDKSampleModel(kfserving.KFModel):
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self):
        f = open('imagenet_classes.txt')
        self.classes = [line.strip() for line in f.readlines()]

        model = models.alexnet(pretrained=True)
        model.eval()
        self.model = model

        self.ready = True

    def predict(self, request: Dict) -> Dict:
        inputs = request["instances"]

        # Input is a data URI, split the header and the base64 encoded image
        header, data = inputs[0].split(",", 1)

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

        percentage = torch.nn.functional.softmax(output, dim=1)[0] * 100

        _, top_5 = torch.topk(output, 5)

        results = {}
        for idx in top_5[0]:
            results[self.classes[idx]] = percentage[idx].item()

        return {"predictions": results}


if __name__ == "__main__":
    model = KFServingSDKSampleModel("kfservingsdksample")
    model.load()
    kfserving.KFServer(workers=1).start([model])
