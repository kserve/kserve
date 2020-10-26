import requests
import json
from matplotlib import pyplot as plt
import numpy as np
from aix360.datasets import MNISTDataset
import time
import sys

if len(sys.argv) < 3:
    raise Exception("No endpoint specified. ")
endpoint = sys.argv[1]
headers = {
    'Host': sys.argv[2]
}

data = MNISTDataset()
test_num = 349
plt_image = data.test_data[test_num]
input_image = np.array([data.test_data[test_num]])
input_label = data.test_labels[test_num]
print(input_image.shape)
input_image = input_image.tolist()
payload = {
    "instances": [input_image, [input_label.argmax().item()]]
}

x = time.time()

res = requests.post(endpoint, json=payload, headers=headers)
print("TIME TAKEN: ", x - time.time())

print(res)
res_json = res.json()

print(res_json["explanations"].keys())
label = input_label.argmax()
pred = res_json["explanations"]["prediction"]
adv_pred = res_json["explanations"]["adversarial_prediction"]
print("Label (", label, ") : Prediction (", pred, ") : Adv Pred (", adv_pred,")")


adv_im = np.asarray(res_json["explanations"]["adversarial_example"])
adv_class = res_json["explanations"]["adversarial_prediction"]
image_class = res_json["explanations"]["prediction"]

print("Checking difference...")
is_diff = False
for x in range(0, len(input_image[0])):
    for y in range(0, len(input_image[0][0])):
        for z in range(0, len(input_image[0][0][0])):
            if(input_image[0][x][y][z] != adv_im[0][x][y][z]):
                is_diff = True
print("is different: ", is_diff)

fig0 = (plt_image[:,:,0] + 0.5) * 255
fig_adv = (adv_im[0,:,:,0] + 0.5) * 255

f, axarr = plt.subplots(1, 2, figsize=(10,10))
axarr[0].set_title("Original (" + str(input_label) + ")")
axarr[1].set_title("Adversarial (" + str(adv_class) + ")")

axarr[0].imshow(fig0, cmap="gray")
axarr[1].imshow(fig_adv, cmap="gray")
plt.show()