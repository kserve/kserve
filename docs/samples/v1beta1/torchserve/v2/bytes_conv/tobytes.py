import base64
import json
import argparse
import uuid

parser = argparse.ArgumentParser()
parser.add_argument("filename", help="converts image to bytes array", type=str)
args = parser.parse_args()

image = open(args.filename, "rb")  # open binary file in read mode
image_read = image.read()
image_64_encode = base64.b64encode(image_read)
bytes_array = image_64_encode.decode("utf-8")
request = {
    "inputs": [{"name": str(uuid.uuid4()), "shape": -1, "datatype": "BYTES", "data": bytes_array}]
}

result_file = "{filename}.{ext}".format(filename=str(args.filename).split(".")[0], ext="json")
with open(result_file, "w") as outfile:
    json.dump(request, outfile, indent=4, sort_keys=True)
