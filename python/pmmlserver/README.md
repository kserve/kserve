# PMML Server

[PMML](https://en.wikipedia.org/wiki/Predictive_Model_Markup_Language) is an XML-based predictive model interchange format conceived by Dr. Robert Lee Grossman, then the director of the National Center for Data Mining at the University of Illinois at Chicago. PMML provides a way for analytic applications to describe and exchange predictive models produced by data mining and machine learning algorithms. 


To start the server locally for development needs, run the following command under this folder in your github repository.

```
pip install -e .
```

The following output indicates a successful install.

```
Requirement already satisfied: kfserving>=0.4.0 in /Users/anyisalin/go/src/github.com/kubeflow/kfserving/python/kfserving (from pmmlserver==0.4.0) (0.4.0)
Requirement already satisfied: pypmml==0.9.7 in /usr/local/Cellar/pyenv/1.0.10/versions/3.8.0/envs/daily/lib/python3.8/site-packages (from pmmlserver==0.4.0) (0.9.7)
Requirement already satisfied: protobuf>=3.12.0 in /usr/local/Cellar/pyenv/1.0.10/versions/3.8.0/envs/daily/lib/python3.8/site-packages (from google-api-core<2.0.0dev,>=1.19.0->google-cloud-core<2.0dev,>=1.4.1->google-cloud-storage>=1.31.0->kfserving>=0.4.0->pmmlserver==0.4.0) (3.13.0)
Requirement already satisfied: pycparser in /usr/local/Cellar/pyenv/1.0.10/versions/3.8.0/envs/daily/lib/python3.8/site-packages (from cffi!=1.11.3,>=1.8->cryptography>=1.1.0->adal>=1.2.2->kfserving>=0.4.0->pmmlserver==0.4.0) (2.19)
Installing collected packages: pmmlserver
  Running setup.py develop for pmmlserver
Successfully installed pmmlserver
```

Once PMML server is up and running, you can check for successful installation by running the following command

```
python3 -m pmmlserver
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT] [--max_buffer_size MAX_BUFFER_SIZE] [--workers WORKERS] --model_dir MODEL_DIR [--model_name MODEL_NAME]
__main__.py: error: the following arguments are required: --model_dir

```

You can now point to your `pmlmserver` model directory and use the server to load the model and test for prediction. Model and associaed model class file can be on local filesystem, S3 compatible object storage, Azure Blob Storage, or Google Cloud Storage. Please follow [this sample](https://github.com/kubeflow/kfserving/tree/master/docs/samples/pmml) to test your server by generating your own model. 

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Requirement already satisfied: kfserving>=0.4.0 in /Users/anyisalin/go/src/github.com/kubeflow/kfserving/python/kfserving (from pmmlserver==0.4.0) (0.4.0)
Requirement already satisfied: pypmml==0.9.7 in /usr/local/Cellar/pyenv/1.0.10/versions/3.8.0/envs/daily/lib/python3.8/site-packages (from pmmlserver==0.4.0) (0.9.7)
Requirement already satisfied: protobuf>=3.12.0 in /usr/local/Cellar/pyenv/1.0.10/versions/3.8.0/envs/daily/lib/python3.8/site-packages (from google-api-core<2.0.0dev,>=1.19.0->google-cloud-core<2.0dev,>=1.4.1->google-cloud-storage>=1.31.0->kfserving>=0.4.0->pmmlserver==0.4.0) (3.13.0)
Requirement already satisfied: pycparser in /usr/local/Cellar/pyenv/1.0.10/versions/3.8.0/envs/daily/lib/python3.8/site-packages (from cffi!=1.11.3,>=1.8->cryptography>=1.1.0->adal>=1.2.2->kfserving>=0.4.0->pmmlserver==0.4.0) (2.19)
Installing collected packages: pmmlserver
  Running setup.py develop for pmmlserver
Successfully installed pmmlserver
```

To run tests, please change the test file to point to your model.pt file. Then run the following command:

```bash
make test
```

The following shows the type of output you should see:

```
pytest -W ignore
=========================================================== test session starts ============================================================
platform darwin -- Python 3.7.3, pytest-4.5.0, py-1.8.0, pluggy-0.11.0
rootdir: /Users/animeshsingh/go/src/github.com/kubeflow/kfserving/python/pmmlserver
plugins: tornasync-0.6.0.post1
collected 1 item                                                                                                                           

pmmlserver/test_model.py .                        
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

You should see an output with an ending similar to this

```bash
Step 11/11 : ENTRYPOINT ["python3", "-m", "pmmlserver"]
 ---> Running in 6bbbdda829ec
Removing intermediate container 6bbbdda829ec
 ---> c5ac6833fdfe
Successfully built c5ac6833fdfe
Successfully tagged gcr.io/kfserving/pmmlserver:latest
```

To push your image to your dockerhub repo,

```bash
docker push docker_user_name/pmmlserver:latest
```
