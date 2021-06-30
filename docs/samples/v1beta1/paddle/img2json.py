#!/usr/bin/python3
import os
import argparse
import json
import cv2
from img_preprocess import preprocess

parser = argparse.ArgumentParser()
parser.add_argument("filename", help="converts image to json request",
                    type=str)
args = parser.parse_args()

input_file = args.filename

img = preprocess(cv2.imread(input_file))

request = {"instances": img.tolist()}

output_file = os.path.splitext(input_file)[0] + '.json'
with open(output_file, 'w') as out:
    json.dump(request, out)
