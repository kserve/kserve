import requests
import json
from matplotlib import pyplot as plt
import numpy as np
from aix360.datasets import MNISTDataset
import time


data = MNISTDataset()
input_image = data.test_data[342]
input_image = input_image.tolist()
input_image = {
    "instances": [input_image]
}

input_image = open('./input.json')
input_image = json.load(input_image)

x = time.time()

res = requests.post('http://artserver.drewbutlerbb4-cluster.sjc03.containers.appdomain.cloud/v1/models/artserver:predict', json=input_image)

print("TIME TAKEN: ", x - time.time())

print(res)
print(res.text)

res_json = res.json()

adv_im = np.asarray(res_json["explanations"]["adversarial_example"])
adv_class = res_json["explanations"]["adversarial_prediction"][0]
image_class = res_json["explanations"]["prediction"][0]
input_image = np.asarray(input_image['instances'][0])

fig0 = (input_image[:,:,0] + 0.5) * 255
fig_adv = (adv_im[0,:,:,0] + 0.5) * 255

f, axarr = plt.subplots(1, 2, figsize=(10,10))
axarr[0].set_title("Original (" + str(image_class) + ")")
axarr[1].set_title("Adversarial (" + str(adv_class) + ")")

axarr[0].imshow(fig0, cmap="gray")
axarr[1].imshow(fig_adv, cmap="gray")
plt.show()