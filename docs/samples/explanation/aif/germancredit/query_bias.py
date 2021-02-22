from aif360.metrics import BinaryLabelDatasetMetric
import sys
from subprocess import PIPE, run
import requests
import json
import yaml
import time

import enum

def parse_events(stdout):
    line_split = stdout.split('☁️  cloudevents.Event\n')

    # Parse the input and output data from the cloud events 
    log_data = []
    for event_iter in range(1, len(line_split)):
        event = line_split[event_iter]

        context_attributes = yaml.safe_load(event[event.index("Context Attributes,")+20: event.index("Extensions,")])
        payload = json.loads(event[event.index("Data,")+6:])
        payload["id"] = context_attributes["id"]
        log_data.append(payload)

    # Pair the input logs with the output logs
    id_map = {}
    paired_logs = []
    for log in log_data:
        if log["id"] in id_map:
            try:
                paired_logs.append([id_map[log["id"]]["instances"], log["predictions"]])
            except:
                paired_logs.append([log["instances"], id_map[log["id"]]["predictions"]])
        else:
            id_map[log["id"]] = log

    # Combine the logs
    log_payload = {"instances":[], "outputs":[]}
    for paired_log in paired_logs:
        log_payload["instances"].extend(paired_log[0])
        log_payload["outputs"].extend(paired_log[1])

    return log_payload

if len(sys.argv) < 3:
    raise Exception("No endpoint specified. ")
endpoint = sys.argv[1]
headers = {
    'Host': sys.argv[2]
}
print("Collecting logs...")

command = ['sh', './get_logs.sh']
result = run(command, stdout=PIPE, stderr=PIPE, universal_newlines=True)
payload = parse_events(result.stdout)

print("Sending bias query...")

x = time.time()

res = requests.post(endpoint, json=payload, headers=headers)

print("TIME TAKEN: ", time.time() - x)

print(res)
if not res.ok:
    res.raise_for_status()
res_json = res.json()
print(res_json)