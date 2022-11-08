import sys
import requests
import json
import numpy as np
import time
import sklearn
import sklearn.datasets
import sklearn.ensemble


np.random.seed(1)
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
parameters = {}
test_num = 15
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
    iris = sklearn.datasets.load_iris()
    train, test, labels_train, labels_test = sklearn.model_selection.train_test_split(
        iris.data,
        iris.target,
        train_size=0.80)
    rf = sklearn.ensemble.RandomForestClassifier(n_estimators=500)
    rf.fit(train, labels_train)

feature_names = iris.feature_names
class_names = iris.target_names.tolist()

input_image = {"instances": test[test_num:test_num+1].tolist(),
               "training_data": train.tolist(),
               "feature_names": feature_names,
               "class_names": class_names}
input_image.update(parameters)
print("Sending Explain Query")
x = time.time()

res = requests.post(endpoint, json=input_image, headers=headers)

print("TIME TAKEN: ", time.time() - x)

if not res.ok:
    res.raise_for_status()
res_json = res.json()
temp = res_json["explanations"]
count = 0


for i in temp.keys():
    print("---------")
    print(class_names[int(i)])
    for j in temp[i]:
        str_tokens = j[0].split('<')
        if(len(str_tokens) == 2):
            feature_num = int(str_tokens[0].strip())
            s1 = feature_names[feature_num] + ' <' + str_tokens[1].strip()
        else:
            feature_num = int(str_tokens[1].strip())
            s1 = feature_names[feature_num] + ' <' + str_tokens[2].strip()
            s1 = str_tokens[0].strip() + ' < ' + s1
        s1 = s1 + '  ,  ' + str(j[1])
        print(s1)
