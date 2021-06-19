import sys
import requests
import json
import time


if len(sys.argv) < 3:
    raise Exception("No endpoint specified. ")
endpoint = sys.argv[1]
headers = {
    'Host': sys.argv[2]
}

payload_file = "input.json"
if len(sys.argv) > 3:
    payload_file = sys.argv[3]

with open(payload_file) as file:
    payload = json.load(file)

print("Sending bias query...")

x = time.time()

res = requests.post(endpoint, json=payload, headers=headers)

print("TIME TAKEN: ", time.time() - x)

print(res)
if not res.ok:
    res.raise_for_status()
res_json = res.json()

# If this is an explanation request
if "metrics" in res_json:
    for metric in res_json["metrics"]:
        print(metric, ": ", res_json["metrics"][metric])
# Else if it is a prediction request
else:
    print(res_json)
