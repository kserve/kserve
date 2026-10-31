# Paddle Server

[Paddle](https://www.paddlepaddle.org.cn/) server is an implementation for serving Paddle models, and provides a Paddle model implementation for prediction, pre and post processing.

To start the server locally for development needs, run the following command under this folder in your github repository.
``` console
make dev_install
```

You can check for successful installation by running the following command
``` console
$ python3 -m paddleserver --help
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT] [--max_buffer_size MAX_BUFFER_SIZE] [--workers WORKERS] --model_dir MODEL_DIR [--model_name MODEL_NAME]

optional arguments:
  -h, --help            show this help message and exit
  --http_port HTTP_PORT
                        The HTTP Port listened to by the model server.
  --grpc_port GRPC_PORT
                        The GRPC Port listened to by the model server.
  --max_buffer_size MAX_BUFFER_SIZE
                        The max buffer size for tornado.
  --workers WORKERS     The number of works to fork
  --model_dir MODEL_DIR
                        A URI pointer to the model directory
  --model_name MODEL_NAME
                        The name that the model is served under.
```
You can now point to your paddle model directory and use the server to load the model and test for prediction. Models can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage.

## Multiple inputs and outputs

For a model with inputs named `ids` and `mask`, V1 accepts column-oriented
named inputs:

```json
{"inputs": {"ids": [[1, 2], [3, 4]], "mask": [[1, 1], [1, 0]]}}
```

Or row-oriented instances:

```json
{"instances": [{"ids": [1, 2], "mask": [1, 1]}, {"ids": [3, 4], "mask": [1, 0]}]}
```

Every required input must be present, with no extra names. V1 input values are
converted to the input types declared by the Paddle model. For multiple outputs,
`predictions` contains one dictionary per batch item, keyed by the model's output
names. These outputs must share a leading batch dimension; their remaining
dimensions may differ.

V2 requests identify each tensor by its model input name, so input order does not
matter. For example:

```json
{
  "inputs": [
    {"name": "mask", "shape": [2, 2], "datatype": "FP32", "data": [1, 1, 1, 0]},
    {"name": "ids", "shape": [2, 2], "datatype": "INT64", "data": [1, 2, 3, 4]}
  ]
}
```

V2 preserves each input's declared datatype and returns each output separately,
with its model output name, shape, and datatype. It supports outputs of different
shapes, requested output selection, and binary responses.

Existing single-input requests can continue to use an arbitrary V2 input name or
an unnamed V1 array. Single-output responses retain the V2 name `output-0` and
the existing V1 predictions array.

## Building your own Paddle Server Docker Image
You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for Paddle in the api directory to point to your own image.

To build your own image, navigate up one directory level to the python directory and run:
```bash
docker build -t ${docker_user_name}/paddleserver -f paddle.Dockerfile .
```

To push your image to your dockerhub repo,
```bash
docker push ${docker_user_name}/paddleserver:latest
```
