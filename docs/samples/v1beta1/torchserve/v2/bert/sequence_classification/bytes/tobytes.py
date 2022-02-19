import uuid
import json
from argparse import ArgumentParser

parser = ArgumentParser()
parser.add_argument("--input_text", help="Input text", type=str, required=True)
parser.add_argument("--result_file", help="Path to result file", default="bert_v2.json", type=str)
args = vars(parser.parse_args())

request = {
  "id": str(uuid.uuid4()),
  "inputs": [{
    "name": str(uuid.uuid4()),
    "shape": -1,
    "datatype": "BYTES",
    "data": args["input_text"]
  }]
}

result_file = args["result_file"]
print("Generating input file: ", result_file)
with open(result_file, "w") as outfile:
    json.dump(request, outfile, indent=4)
