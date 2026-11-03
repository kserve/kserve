import sys
import requests
import json
from matplotlib import pyplot as plt
import numpy as np
from sklearn.datasets import fetch_20newsgroups
import time

print("************************************************************")
print("************************************************************")
print("************************************************************")
print("starting query")

if len(sys.argv) < 3:
    raise Exception("No endpoint specified. ")
endpoint = sys.argv[1]
headers = {"Host": sys.argv[2]}
parameters = {}
test_num = 1002
is_file = False
if len(sys.argv) > 3:
    try:
        test_num = int(sys.argv[3])
    except Exception:  # pylint: disable=broad-except
        is_file = True

    if len(sys.argv) > 4:
        try:
            parameters = json.loads(sys.argv[4])
        except Exception:  # pylint: disable=broad-except
            raise Exception("Failed to convert to json format. ")
if is_file:
    inputs = open(sys.argv[2])
    inputs = json.load(inputs)
    actual = "unk"
else:
    data = fetch_20newsgroups()  # input dataset
    inputs = data.data[test_num]
    labels = data.target[test_num]
    actual = data.target_names[labels]
    # to do list is below
input_text = {"instances": [inputs]}
input_text.update(parameters)
print("Sending Explain Query")

x = time.time()

res = requests.post(endpoint, json=input_text, headers=headers)

print("TIME TAKEN: ", time.time() - x)

print(res)
if not res.ok:
    res.raise_for_status()

res_json = res.json()
temp = res_json["explanations"]

#  get class name from dataset
nameset = fetch_20newsgroups(subset="train")
class_names = [
    x.split(".")[-1] if "misc" not in x else ".".join(x.split(".")[-2:])
    for x in nameset.target_names
]

#  print detailed values
for feature_label in temp.keys():
    print("\nExplanation of " + class_names[int(feature_label)] + ":")
    for composition in temp[feature_label]:
        print(composition)

#  draw bar chart
features = []
composition_value = []
subplot_count = 1
for feature_label in temp.keys():
    plt.subplot(3, 4, subplot_count)
    plt.title(class_names[int(feature_label)])
    for i in temp[feature_label]:
        features.append(i[0])
        composition_value.append(i[1])
    x_axis = np.arange(len(features))
    plt.barh(x_axis, composition_value)
    plt.yticks(x_axis, features)
    subplot_count += 1
    features = []
    composition_value = []
plt.subplots_adjust(left=0.125, bottom=0.1, right=0.9, top=0.9, wspace=0.5, hspace=0.5)
plt.show()
