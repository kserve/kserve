from subprocess import PIPE, run
import json
import yaml


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
            except Exception:  # pylint: disable=broad-except
                paired_logs.append([log["instances"], id_map[log["id"]]["predictions"]])
        else:
            id_map[log["id"]] = log

    # Combine the logs
    log_payload = {"instances": [], "outputs": []}
    for paired_log in paired_logs:
        log_payload["instances"].extend(paired_log[0])
        log_payload["outputs"].extend(paired_log[1])

    return log_payload


command = ['sh', './get_logs.sh']
result = run(command, stdout=PIPE, stderr=PIPE, universal_newlines=True)
json_data = parse_events(result.stdout)

with open('data.json', 'w') as f:
    json.dump(json_data, f)
