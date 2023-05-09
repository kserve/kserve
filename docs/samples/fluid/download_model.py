# Copyright 2023, SAP SE or an SAP affiliate company and KServe contributors

import argparse
from pathlib import Path
from huggingface_hub import snapshot_download


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('--model_name', default="bigscience/bloom-560m", help='model name from huggingface')
    parser.add_argument('--model_dir', default="models", help='dir to download the model')
    parser.add_argument('--revision', default="main", help='revision of the model')
    args = vars(parser.parse_args())

    model_name = args["model_name"]
    revision = args["revision"]

    model_dir = Path(args["model_dir"])
    model_dir.mkdir(exist_ok=True)

    snapshot_download(repo_id=model_name, revision=revision, cache_dir=model_dir)

    # reference: https://aws.amazon.com/de/blogs/machine-learning/deploy-bloom-176b-and-opt-30b-on-amazon-sagemaker-with-large-model-inference-deep-learning-containers-and-deepspeed/ # noqa: E501
    output_dir = list(model_dir.glob("**/snapshots/*"))[0]
    print(f"export output_dir={output_dir}")
