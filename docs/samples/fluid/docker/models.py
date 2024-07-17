# Copyright 2023, SAP SE or an SAP affiliate company and KServe contributors

import os

from transformers import AutoTokenizer, AutoModelForCausalLM
import torch

from flask import Flask, request
import logging
from signal import signal, SIGINT

app = Flask(__name__)
logging.basicConfig(
    level=logging.DEBUG,
    format="%(asctime)s %(levelname)s %(name)s %(threadName)s : %(message)s",
)

model, tokenizer = None, None

MODEL_URL = os.environ.get("MODEL_URL", "/mnt/models")
MODEL_NAME = os.environ.get("MODEL_NAME", "custom")
LOAD_IN_8BIT = os.environ.get("LOAD_IN_8BIT", "False") == "True"
GPU_ENABLED = os.environ.get("GPU_ENABLED", "False") == "True"


def handler(signal_received, frame):
    # SIGINT or  ctrl-C detected, exit without error
    exit(0)


def check_gpu():
    app.logger.info(f"CUDA is available: {torch.cuda.is_available()}")

    # setting device on GPU if available, else CPU
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    app.logger.info(f"Using device: {device}")

    # additional Info when using cuda
    if device.type == "cuda":
        gpu_count = torch.cuda.device_count()
        app.logger.info(f"Device Count: {gpu_count}")
        for i in range(gpu_count):
            app.logger.info(f"Device ID: {i}")
            app.logger.info(f"Device Name: {torch.cuda.get_device_name(i)}")
            app.logger.info("Memory Usage:")
            app.logger.info(
                f"Allocated: {round(torch.cuda.memory_allocated(i)/1024**3,1)} GB"
            )
            app.logger.info(
                f"Cached: {round(torch.cuda.memory_reserved(i)/1024**3,1)} GB"
            )


@app.before_first_request
def init():

    check_gpu()

    global model, tokenizer
    if model is None or tokenizer is None:
        app.logger.info(f"Loading Model from {MODEL_URL}")
        app.logger.info(f"Loading Model in 8 bit: {LOAD_IN_8BIT}")

        tokenizer = AutoTokenizer.from_pretrained(MODEL_URL)
        model = AutoModelForCausalLM.from_pretrained(
            MODEL_URL, device_map="auto", load_in_8bit=LOAD_IN_8BIT
        )
        app.logger.info("Model loaded successfully")


@app.route("/v1/models/{}:predict".format(MODEL_NAME), methods=["POST"])
def predict():
    """
    Perform an inference on the model created in initialize

    Returns:
        String response of the prompt for the given test data
    """
    global model, tokenizer
    input_data = dict(request.json)

    if "prompt" not in input_data:
        return "Prompt not found", 400

    prompt = input_data["prompt"]
    result_length = input_data.get("result_length", 50)

    if GPU_ENABLED:
        input = tokenizer(prompt, return_tensors="pt").to("cuda")
    else:
        input = tokenizer(prompt, return_tensors="pt")

    result = tokenizer.batch_decode(
        model.generate(input["input_ids"], max_length=result_length)[0],
        skip_special_tokens=True,
    )

    output = {"result": "".join(result)}
    return output


if __name__ == "__main__":
    signal(SIGINT, handler)
    app.run(host="0.0.0.0", debug=True, port=8080)
