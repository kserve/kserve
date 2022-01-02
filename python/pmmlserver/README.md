# PMML Server

[PMML](https://en.wikipedia.org/wiki/Predictive_Model_Markup_Language) is an XML-based predictive model interchange format conceived by Dr. Robert Lee Grossman, then the director of the National Center for Data Mining at the University of Illinois at Chicago. PMML provides a way for analytic applications to describe and exchange predictive models produced by data mining and machine learning algorithms. 


To start the server locally for development needs, run the following command under this folder in your github repository.

```
pip install -e .
```

Once PMML server is up and running, you can check for successful installation by running the following command

```
python3 -m pmmlserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT] [--max_buffer_size MAX_BUFFER_SIZE] [--workers WORKERS] --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir

```

You can now point to your `pmmlserver` model directory and use the server to load the model and test for prediction. Model and associated model class file can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kserve/kserve/tree/master/docs/samples/v1beta1/pmml) to test your server by generating your own model. 

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

To run tests, please change the test file to point to your model.pt file. Then run the following command:

```bash
make test
```

To run static type checks:

```bash
mypy --ignore-missing-imports pmmlserver
```

An empty result will indicate success.

## Building your own PMML server Docker Image

You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for pmml in the api directory to point to your own image.

To build your own image, navigate up one directory level to the `python` directory and run:

```bash
docker build . -t docker_user_name/pmmlserver:latest -f pmml.Dockerfile
```

To push your image to your dockerhub repo,

```bash
docker push docker_user_name/pmmlserver:latest
```
