# Paddle Server

[Paddle](https://www.paddlepaddle.org.cn/) server is an implementation of KFServing for serving Paddle models, and provides a Paddle model implementation for prediction, pre and post processing.

To start the server locally for development needs, run the following command under this folder in your github repository.
``` console
pip install -e .
```

The following output indicates a successful install.
``` console
Obtaining file:///home/zzy/src/kubeflow/kfserving/python/paddleserver
Requirement already satisfied: kfserving>=0.5.1 in /home/zzy/src/kubeflow/kfserving/python/kfserving (from paddleserver==0.5.0) (0.6.0rc0)
Requirement already satisfied: paddlepaddle>=2.0.2 in /home/zzy/.local/lib/python3.8/site-packages (from paddleserver==0.5.0) (2.0.2)
Requirement already satisfied: adal>=1.2.2 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.2.7)
Collecting argparse>=1.4.0
  Using cached argparse-1.4.0-py2.py3-none-any.whl (23 kB)
Requirement already satisfied: avro>=1.10.1 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.10.2)
Requirement already satisfied: azure-storage-blob<=2.1.0,>=1.3.0 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (2.1.0)
Requirement already satisfied: boto3>=1.17.32 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.17.69)
Requirement already satisfied: botocore>=1.20.32 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.20.69)
Requirement already satisfied: certifi>=14.05.14 in /usr/lib/python3/dist-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (2019.11.28)
Requirement already satisfied: cloudevents>=1.2.0 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.2.0)
Requirement already satisfied: google-cloud-storage>=1.31.0 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.38.0)
Requirement already satisfied: kubernetes>=12.0.0 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (12.0.1)
Requirement already satisfied: minio<7.0.0,>=4.0.9 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (6.0.2)
Requirement already satisfied: numpy>=1.17.3 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.19.5)
Requirement already satisfied: python_dateutil>=2.5.3 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (2.8.1)
Requirement already satisfied: setuptools>=21.0.0 in /usr/lib/python3/dist-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (45.2.0)
Requirement already satisfied: six>=1.15 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.15.0)
Requirement already satisfied: table_logger>=0.3.5 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (0.3.6)
Requirement already satisfied: tornado>=6.0.0 in /home/zzy/.local/lib/python3.8/site-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (6.1)
Requirement already satisfied: urllib3>=1.15.1 in /usr/lib/python3/dist-packages (from kfserving>=0.5.1->paddleserver==0.5.0) (1.25.8)
Requirement already satisfied: PyJWT<3,>=1.0.0 in /usr/lib/python3/dist-packages (from adal>=1.2.2->kfserving>=0.5.1->paddleserver==0.5.0) (1.7.1)
Requirement already satisfied: cryptography>=1.1.0 in /usr/lib/python3/dist-packages (from adal>=1.2.2->kfserving>=0.5.1->paddleserver==0.5.0) (2.8)
Requirement already satisfied: requests<3,>=2.0.0 in /usr/lib/python3/dist-packages (from adal>=1.2.2->kfserving>=0.5.1->paddleserver==0.5.0) (2.22.0)
Requirement already satisfied: azure-storage-common~=2.1 in /home/zzy/.local/lib/python3.8/site-packages (from azure-storage-blob<=2.1.0,>=1.3.0->kfserving>=0.5.1->paddleserver==0.5.0) (2.1.0)
Requirement already satisfied: azure-common>=1.1.5 in /home/zzy/.local/lib/python3.8/site-packages (from azure-storage-blob<=2.1.0,>=1.3.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.1.27)
Requirement already satisfied: s3transfer<0.5.0,>=0.4.0 in /home/zzy/.local/lib/python3.8/site-packages (from boto3>=1.17.32->kfserving>=0.5.1->paddleserver==0.5.0) (0.4.2)
Requirement already satisfied: jmespath<1.0.0,>=0.7.1 in /home/zzy/.local/lib/python3.8/site-packages (from boto3>=1.17.32->kfserving>=0.5.1->paddleserver==0.5.0) (0.10.0)
Requirement already satisfied: deprecation<3.0,>=2.0 in /home/zzy/.local/lib/python3.8/site-packages (from cloudevents>=1.2.0->kfserving>=0.5.1->paddleserver==0.5.0) (2.1.0)
Requirement already satisfied: packaging in /home/zzy/.local/lib/python3.8/site-packages (from deprecation<3.0,>=2.0->cloudevents>=1.2.0->kfserving>=0.5.1->paddleserver==0.5.0) (20.9)
Requirement already satisfied: google-resumable-media<2.0dev,>=1.2.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.2.0)
Requirement already satisfied: google-auth<2.0dev,>=1.11.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.24.0)
Requirement already satisfied: google-cloud-core<2.0dev,>=1.4.1 in /home/zzy/.local/lib/python3.8/site-packages (from google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.6.0)
Requirement already satisfied: pyasn1-modules>=0.2.1 in /usr/lib/python3/dist-packages (from google-auth<2.0dev,>=1.11.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (0.2.1)
Requirement already satisfied: cachetools<5.0,>=2.0.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-auth<2.0dev,>=1.11.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (4.2.0)
Requirement already satisfied: rsa<5,>=3.1.4 in /home/zzy/.local/lib/python3.8/site-packages (from google-auth<2.0dev,>=1.11.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (4.7)
Requirement already satisfied: google-api-core<2.0.0dev,>=1.21.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-cloud-core<2.0dev,>=1.4.1->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.26.3)
Requirement already satisfied: pytz in /home/zzy/.local/lib/python3.8/site-packages (from google-api-core<2.0.0dev,>=1.21.0->google-cloud-core<2.0dev,>=1.4.1->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (2021.1)
Requirement already satisfied: googleapis-common-protos<2.0dev,>=1.6.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-api-core<2.0.0dev,>=1.21.0->google-cloud-core<2.0dev,>=1.4.1->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.53.0)
Requirement already satisfied: protobuf>=3.12.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-api-core<2.0.0dev,>=1.21.0->google-cloud-core<2.0dev,>=1.4.1->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (3.14.0)
Requirement already satisfied: google-crc32c<2.0dev,>=1.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-resumable-media<2.0dev,>=1.2.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.1.2)
Requirement already satisfied: cffi>=1.0.0 in /home/zzy/.local/lib/python3.8/site-packages (from google-crc32c<2.0dev,>=1.0->google-resumable-media<2.0dev,>=1.2.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.14.5)
Requirement already satisfied: pycparser in /home/zzy/.local/lib/python3.8/site-packages (from cffi>=1.0.0->google-crc32c<2.0dev,>=1.0->google-resumable-media<2.0dev,>=1.2.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (2.20)
Requirement already satisfied: pyyaml>=3.12 in /usr/lib/python3/dist-packages (from kubernetes>=12.0.0->kfserving>=0.5.1->paddleserver==0.5.0) (5.3.1)
Requirement already satisfied: requests-oauthlib in /home/zzy/.local/lib/python3.8/site-packages (from kubernetes>=12.0.0->kfserving>=0.5.1->paddleserver==0.5.0) (1.3.0)
Requirement already satisfied: websocket-client!=0.40.0,!=0.41.*,!=0.42.*,>=0.32.0 in /home/zzy/.local/lib/python3.8/site-packages (from kubernetes>=12.0.0->kfserving>=0.5.1->paddleserver==0.5.0) (0.57.0)
Requirement already satisfied: configparser in /home/zzy/.local/lib/python3.8/site-packages (from minio<7.0.0,>=4.0.9->kfserving>=0.5.1->paddleserver==0.5.0) (5.0.2)
Requirement already satisfied: pyparsing>=2.0.2 in /home/zzy/.local/lib/python3.8/site-packages (from packaging->deprecation<3.0,>=2.0->cloudevents>=1.2.0->kfserving>=0.5.1->paddleserver==0.5.0) (2.4.7)
Requirement already satisfied: decorator in /home/zzy/.local/lib/python3.8/site-packages (from paddlepaddle>=2.0.2->paddleserver==0.5.0) (4.4.2)
Requirement already satisfied: Pillow in /home/zzy/.local/lib/python3.8/site-packages (from paddlepaddle>=2.0.2->paddleserver==0.5.0) (8.1.0)
Requirement already satisfied: astor in /home/zzy/.local/lib/python3.8/site-packages (from paddlepaddle>=2.0.2->paddleserver==0.5.0) (0.8.1)
Requirement already satisfied: gast>=0.3.3 in /home/zzy/.local/lib/python3.8/site-packages (from paddlepaddle>=2.0.2->paddleserver==0.5.0) (0.3.3)
Requirement already satisfied: pyasn1>=0.1.3 in /usr/lib/python3/dist-packages (from rsa<5,>=3.1.4->google-auth<2.0dev,>=1.11.0->google-cloud-storage>=1.31.0->kfserving>=0.5.1->paddleserver==0.5.0) (0.4.2)
Requirement already satisfied: oauthlib>=3.0.0 in /usr/lib/python3/dist-packages (from requests-oauthlib->kubernetes>=12.0.0->kfserving>=0.5.1->paddleserver==0.5.0) (3.1.0)
Installing collected packages: argparse, paddleserver
  Attempting uninstall: paddleserver
    Found existing installation: paddleserver 0.5.0
    Uninstalling paddleserver-0.5.0:
      Successfully uninstalled paddleserver-0.5.0
  Running setup.py develop for paddleserver
Successfully installed argparse-1.4.0 paddleserver
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
