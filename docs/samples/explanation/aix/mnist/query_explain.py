import os
import sys
import requests
import json
from matplotlib import pyplot as plt
import numpy as np
from aix360.datasets import MNISTDataset
from keras.applications import inception_v3 as inc_net
from keras.preprocessing import image
from keras.applications.imagenet_utils import decode_predictions
import time
from skimage.color import gray2rgb, rgb2gray, label2rgb # since the code wants color images

print('************************************************************')
print('************************************************************')
print('************************************************************')
print("starting query")

if len(sys.argv) < 3:
    raise Exception("No endpoint specified. ")
endpoint = sys.argv[1]
headers = {
    'Host': sys.argv[2]
}
test_num = 1002
is_file = False
if len(sys.argv) > 3:
    try:
        test_num = int(sys.argv[2])
    except:
        is_file = True

if is_file:
    inputs = open(sys.argv[2])
    inputs = json.load(inputs)
    actual = "unk"
else:
    data = MNISTDataset()
    inputs = data.test_data[test_num]
    labels = data.test_labels[test_num]
    actual = 0
    for x in range(1, len(labels)):
        if labels[x] != 0:
            actual = x
    inputs = gray2rgb(inputs.reshape((-1, 28, 28)))
    inputs = np.reshape(inputs, (28,28,3))
input_image = {"instances": [inputs.tolist()]}
print("Sending Explain Query")

x = time.time()

res = requests.post(endpoint, json=input_image, headers=headers)

print("TIME TAKEN: ", time.time() - x)

print(res)
if not res.ok:
    res.raise_for_status()
res_json = res.json()
temp = np.array(res_json["explanations"]["temp"])
masks = np.array(res_json["explanations"]["masks"])
top_labels = np.array(res_json["explanations"]["top_labels"])

fig, m_axs = plt.subplots(2,5, figsize = (12,6))
for i, c_ax in enumerate(m_axs.flatten()):
    mask = masks[i]
    c_ax.imshow(label2rgb(mask, temp, bg_label = 0), interpolation = 'nearest')
    c_ax.set_title('Positive for {}\nActual {}'.format(top_labels[i], actual))
    c_ax.axis('off')
plt.show()
