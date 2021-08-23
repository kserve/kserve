#!/usr/bin/python3
"""
The script takes text or image file as input and generates json input with 
tensor inputs for kfserving v2 protocol.
"""
import json
import uuid
import numpy as np
import argparse
from utils import check_image_with_pil, _to_datatype
from PIL import Image
from torchvision import transforms

parser = argparse.ArgumentParser()
parser.add_argument('filename', help='input filename')
args = parser.parse_args()
args = vars(args)
filename = args["filename"]

if check_image_with_pil(filename):
    image = Image.open(filename)  # PIL's JpegImageFile format (size=(W,H))
    tran = transforms.ToTensor(
    )  # Convert the numpy array or PIL.Image read image to (C, H, W) Tensor format and /255 normalize to [0, 1.0]
    data = tran(image)
else:
    with open(filename, 'r') as fp:
        text = fp.read()
    data = list(bytes(text.encode()))

data = np.array(data)
data_shape = list(data.shape)
data_type = data.dtype
request = {
    "inputs": [{
        "name": str(uuid.uuid4()),
        "shape": data_shape,
        "datatype": _to_datatype(data_type),
        "data": np.round(data, 4).tolist()
    }]
}
with open('input.json', 'w') as outfile:
    json.dump(request, outfile)
