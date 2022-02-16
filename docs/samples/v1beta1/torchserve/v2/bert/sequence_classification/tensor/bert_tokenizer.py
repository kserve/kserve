import uuid
import json
from argparse import ArgumentParser
from transformers import AutoTokenizer

parser = ArgumentParser()
parser.add_argument("--input_text", help="Input text", type=str, required=True)
parser.add_argument("--model_name", help="bert model name", default="bert-base-uncased", type=str)
parser.add_argument("--do_lower_case", help="Use lower case", default=True, type=bool)
parser.add_argument("--max_length", help="Max length of the string", default=150, type=int)
parser.add_argument("--result_file", help="Path to result file", default="bert_v2.json", type=str)
args = vars(parser.parse_args())

tokenizer = AutoTokenizer.from_pretrained(args["model_name"], do_lower_case=True)

print("Tokenizing input")
tokenized_input = tokenizer.encode_plus(
    args["input_text"],
    max_length=args["max_length"],
    pad_to_max_length=True,
    add_special_tokens=True,
    return_tensors="pt",
)

input_ids = tokenized_input["input_ids"]
attention_mask = tokenized_input["attention_mask"]

request = {
    "id": str(uuid.uuid4()),
    "inputs": [
        {
            "name": "input_ids",
            "shape": input_ids.shape[1],
            "datatype": "INT64",
            "data": input_ids[0].tolist(),
        },
        {
            "name": "attention_masks",
            "shape": attention_mask.shape[1],
            "datatype": "INT64",
            "data": attention_mask[0].tolist(),
        },
    ],
}

result_file = args["result_file"]
print("Generating input file: ", result_file)
with open(result_file, "w") as outfile:
    json.dump(request, outfile)
