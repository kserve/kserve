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
is_file = False
if len(sys.argv) > 3:
    try:
        test_num = int(sys.argv[3])
    except Exception:  # pylint: disable=broad-except
        is_file = True

if is_file:
    inputs = open(sys.argv[3])
    payload = json.load(inputs)
    input_image = payload["instances"][0]
    plt_image = np.array(input_image[0])
    label = payload["instances"][1][0]
else:
    plt_image = data.test_data[test_num]
    input_image = np.array([data.test_data[test_num]])
    input_label = data.test_labels[test_num]
    input_image = input_image.tolist()
    label = input_label.argmax()
    payload = {
        "instances": [input_image, [input_label.argmax().item()]]
    }

x = time.time()

res = requests.post(endpoint, json=payload, headers=headers)
print("TIME TAKEN: ", time.time() - x)

print(res)
res_json = res.json()

adv_im = np.asarray(res_json["explanations"]["adversarial_example"])
adv_class = res_json["explanations"]["adversarial_prediction"]
image_class = res_json["explanations"]["prediction"]

is_diff = False
for x in range(0, len(input_image[0])):
    for y in range(0, len(input_image[0][0])):
        for z in range(0, len(input_image[0][0][0])):
            if input_image[0][x][y][z] != adv_im[0][x][y][z]:
                is_diff = True

if is_diff:
    print("Found an adversarial example.")
    fig0 = (plt_image[:, :, 0] + 0.5) * 255
    fig_adv = (adv_im[0, :, :, 0] + 0.5) * 255

    f, axarr = plt.subplots(1, 2, figsize=(10, 10))
    axarr[0].set_title("Original (" + str(label) + ")")
    axarr[1].set_title("Adversarial (" + str(adv_class) + ")")

    axarr[0].imshow(fig0, cmap="gray")
    axarr[1].imshow(fig_adv, cmap="gray")
    plt.show()
else:
    print("Unable to find an adversarial example.")
