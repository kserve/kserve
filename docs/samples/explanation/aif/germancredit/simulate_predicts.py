import sys
import json
import time
import requests

if len(sys.argv) < 3:
    raise Exception("No endpoint specified. ")
endpoint = sys.argv[1]
headers = {
    'Host': sys.argv[2]
}

with open('input.json') as file:
    sample_file = json.load(file)
inputs = sample_file["instances"]

# Split inputs into chunks of size 15 and send them to the predict server
print("Sending prediction requests...")
time_before = time.time()
res = requests.post(endpoint, json={"instances": inputs}, headers=headers)

for x in range(0, len(inputs), 15):
    query_inputs = inputs[x: x+20]
    payload = {"instances": query_inputs}

    res = requests.post(endpoint, json=payload, headers=headers)
    print(res)
    if not res.ok:
        res.raise_for_status()

print("TIME TAKEN: ", time.time() - time_before)
print("Last response: ", res.json())
