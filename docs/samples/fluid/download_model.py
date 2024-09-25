# Copyright 2023, SAP SE or an SAP affiliate company and KServe contributors

import argparse
from pathlib import Path
from huggingface_hub import snapshot_download


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--model_name",
        default="bigscience/bloom-560m",
        help="model name from huggingface",
    )
    parser.add_argument(
        "--model_dir",
        default="./models",
        help="dir to download the model",
    )
    parser.add_argument(
        "--revision",
        default="main",
        help="revision of the model",
    )

    args = vars(parser.parse_args())
    model_name = args["model_name"]
    revision = args["revision"]
    out_dir = args["model_dir"]

    tmp_model_name = model_name.replace("/", "--")

    model_dir = Path(out_dir, f"models--{tmp_model_name}", "snapshots", revision)

    # check the model repo and update it accordingly
    allow_patterns = ["*.json", "*.safetensors", "*.model"]
    # here safetensors is the preferred format.
    ignore_patterns = ["*.msgpack", "*.h5", "*.bin"]

    # set the path to download the model
    models_path = Path(model_dir)
    models_path.mkdir(parents=True, exist_ok=True)

    # download the snapshot
    output_dir = snapshot_download(
        repo_id=model_name,
        revision=revision,
        allow_patterns=allow_patterns,
        ignore_patterns=ignore_patterns,
        local_dir=models_path,
    )
    print(output_dir)
