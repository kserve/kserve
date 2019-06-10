# Alibi Model Explainer

[Alibi](https://github.com/SeldonIO/alibi) server is an implementation of a KFServing for providing black box model explanation for KFServer models.

To start the server locally for development needs, run the following command under this folder in your github repository. 

```
pip install -e .
```

The following output indicates a successful install.

```
Obtaining file:///home/clive/go/src/github.com/kubeflow/kfserving/python/alibiexplainer
Requirement already satisfied: kfserver==0.1.0 in /home/clive/go/src/github.com/kubeflow/kfserving/python/kfserving (from alibiexplainer==0.1.0) (0.1.0)
Requirement already satisfied: alibi==0.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.2.0)
Requirement already satisfied: scikit-learn>=0.20.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.20.3)
Requirement already satisfied: argparse>=1.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (1.4.0)
Requirement already satisfied: requests>=2.22.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (2.22.0)
Requirement already satisfied: joblib>=0.13.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.13.2)
Requirement already satisfied: pandas>=0.24.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.24.2)
Requirement already satisfied: numpy>=1.16.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (1.16.3)
Requirement already satisfied: tornado>=1.4.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (6.0.2)
Requirement already satisfied: minio>=4.0.9 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (4.0.17)
Requirement already satisfied: google-cloud-storage>=1.16.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (1.16.1)
Requirement already satisfied: keras in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (2.2.4)
Requirement already satisfied: scikit-image in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (0.15.0)
Requirement already satisfied: tensorflow in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (1.13.1)
Requirement already satisfied: opencv-python in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (4.1.0.25)
Requirement already satisfied: seaborn in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (0.9.0)
Requirement already satisfied: beautifulsoup4 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (4.7.1)
Requirement already satisfied: spacy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibi==0.2.0->alibiexplainer==0.1.0) (2.1.4)
Requirement already satisfied: scipy>=0.13.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-learn>=0.20.3->alibiexplainer==0.1.0) (1.2.1)
Requirement already satisfied: certifi>=2017.4.17 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (2019.3.9)
Requirement already satisfied: idna<2.9,>=2.5 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (2.8)
Requirement already satisfied: urllib3!=1.25.0,!=1.25.1,<1.26,>=1.21.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (1.25.3)
Requirement already satisfied: chardet<3.1.0,>=3.0.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (3.0.4)
Requirement already satisfied: pytz>=2011k in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pandas>=0.24.2->alibiexplainer==0.1.0) (2019.1)
Requirement already satisfied: python-dateutil>=2.5.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pandas>=0.24.2->alibiexplainer==0.1.0) (2.8.0)
Requirement already satisfied: google-cloud-core<2.0dev,>=1.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.0.1)
Requirement already satisfied: google-resumable-media>=0.3.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (0.3.2)
Requirement already satisfied: google-auth>=1.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.6.3)
Requirement already satisfied: pyyaml in /home/clive/.local/lib/python3.7/site-packages (from keras->alibi==0.2.0->alibiexplainer==0.1.0) (3.13)
Requirement already satisfied: h5py in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from keras->alibi==0.2.0->alibiexplainer==0.1.0) (2.9.0)
Requirement already satisfied: six>=1.9.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from keras->alibi==0.2.0->alibiexplainer==0.1.0) (1.12.0)
Requirement already satisfied: keras-applications>=1.0.6 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from keras->alibi==0.2.0->alibiexplainer==0.1.0) (1.0.8)
Requirement already satisfied: keras-preprocessing>=1.0.5 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from keras->alibi==0.2.0->alibiexplainer==0.1.0) (1.1.0)
Requirement already satisfied: networkx>=2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (2.3)
Requirement already satisfied: matplotlib!=3.0.0,>=2.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (3.1.0)
Requirement already satisfied: imageio>=2.0.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (2.5.0)
Requirement already satisfied: PyWavelets>=0.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (1.0.3)
Requirement already satisfied: pillow>=4.3.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (6.0.0)
Requirement already satisfied: absl-py>=0.1.6 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (0.7.1)
Requirement already satisfied: wheel>=0.26 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (0.33.1)
Requirement already satisfied: tensorflow-estimator<1.14.0rc0,>=1.13.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (1.13.0)
Requirement already satisfied: tensorboard<1.14.0,>=1.13.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (1.13.1)
Requirement already satisfied: astor>=0.6.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (0.8.0)
Requirement already satisfied: gast>=0.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (0.2.2)
Requirement already satisfied: termcolor>=1.1.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (1.1.0)
Requirement already satisfied: protobuf>=3.6.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (3.8.0)
Requirement already satisfied: grpcio>=1.8.6 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (1.21.1)
Requirement already satisfied: soupsieve>=1.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from beautifulsoup4->alibi==0.2.0->alibiexplainer==0.1.0) (1.9.1)
Requirement already satisfied: thinc<7.1.0,>=7.0.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (7.0.4)
Requirement already satisfied: preshed<2.1.0,>=2.0.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (2.0.1)
Requirement already satisfied: blis<0.3.0,>=0.2.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (0.2.4)
Requirement already satisfied: murmurhash<1.1.0,>=0.28.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (1.0.2)
Requirement already satisfied: jsonschema<3.1.0,>=2.6.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (3.0.1)
Requirement already satisfied: plac<1.0.0,>=0.9.6 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (0.9.6)
Requirement already satisfied: cymem<2.1.0,>=2.0.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (2.0.2)
Requirement already satisfied: wasabi<1.1.0,>=0.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (0.2.2)
Requirement already satisfied: srsly<1.1.0,>=0.0.5 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from spacy->alibi==0.2.0->alibiexplainer==0.1.0) (0.0.6)
Requirement already satisfied: google-api-core<2.0.0dev,>=1.11.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-core<2.0dev,>=1.0.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.11.1)
Requirement already satisfied: rsa>=3.1.4 in /home/clive/.local/lib/python3.7/site-packages (from google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (3.4.2)
Requirement already satisfied: pyasn1-modules>=0.2.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (0.2.5)
Requirement already satisfied: cachetools>=2.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (3.1.1)
Requirement already satisfied: decorator>=4.3.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from networkx>=2.0->scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (4.4.0)
Requirement already satisfied: kiwisolver>=1.0.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from matplotlib!=3.0.0,>=2.0.0->scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (1.1.0)
Requirement already satisfied: cycler>=0.10 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from matplotlib!=3.0.0,>=2.0.0->scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (0.10.0)
Requirement already satisfied: pyparsing!=2.0.4,!=2.1.2,!=2.1.6,>=2.0.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from matplotlib!=3.0.0,>=2.0.0->scikit-image->alibi==0.2.0->alibiexplainer==0.1.0) (2.4.0)
Requirement already satisfied: mock>=2.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow-estimator<1.14.0rc0,>=1.13.0->tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (3.0.5)
Requirement already satisfied: markdown>=2.6.8 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorboard<1.14.0,>=1.13.0->tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (3.1.1)
Requirement already satisfied: werkzeug>=0.11.15 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorboard<1.14.0,>=1.13.0->tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (0.15.4)
Requirement already satisfied: setuptools in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from protobuf>=3.6.1->tensorflow->alibi==0.2.0->alibiexplainer==0.1.0) (41.0.1)
Requirement already satisfied: tqdm<5.0.0,>=4.10.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from thinc<7.1.0,>=7.0.2->spacy->alibi==0.2.0->alibiexplainer==0.1.0) (4.32.1)
Requirement already satisfied: attrs>=17.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from jsonschema<3.1.0,>=2.6.0->spacy->alibi==0.2.0->alibiexplainer==0.1.0) (19.1.0)
Requirement already satisfied: pyrsistent>=0.14.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from jsonschema<3.1.0,>=2.6.0->spacy->alibi==0.2.0->alibiexplainer==0.1.0) (0.15.1)
Requirement already satisfied: googleapis-common-protos!=1.5.4,<2.0dev,>=1.5.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-api-core<2.0.0dev,>=1.11.0->google-cloud-core<2.0dev,>=1.0.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.6.0)
Requirement already satisfied: pyasn1>=0.1.3 in /home/clive/.local/lib/python3.7/site-packages (from rsa>=3.1.4->google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (0.4.5)
Installing collected packages: alibiexplainer
  Found existing installation: alibiexplainer 0.1.0
      Uninstalling alibiexplainer-0.1.0:
            Successfully uninstalled alibiexplainer-0.1.0
	      Running setup.py develop for alibiexplainer
	      Successfully installed alibiexplainer
```

You can check for successful installation by running the following command

```
python3 -m alibiexplainer
usage: alibiexplainer [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                      [--protocol {tensorflow.http,seldon.http}] --model_url
		      MODEL_URL [--method {ExplainerMethod.anchor_tabular}]
alibiexplainer: error: the following arguments are required: --model_url
```

## Samples

To run a local example follow the [income classifier explanation sample](./samples/income/README.md).

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
Requirement already satisfied: kfserver==0.1.0 in /home/clive/go/src/github.com/kubeflow/kfserving/python/kfserving (from alibiexplainer==0.1.0) (0.1.0)
Requirement already satisfied: scikit-learn==0.20.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.20.3)
Requirement already satisfied: argparse>=1.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (1.4.0)
Requirement already satisfied: seldon-core>=0.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.3.0)
Requirement already satisfied: requests>=2.22.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (2.22.0)
Requirement already satisfied: pytest in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (4.4.2)
Requirement already satisfied: pytest-tornasync in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.6.0.post1)
Requirement already satisfied: mypy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from alibiexplainer==0.1.0) (0.701)
Requirement already satisfied: tornado>=1.4.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (6.0.2)
Requirement already satisfied: minio>=4.0.9 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (4.0.17)
Requirement already satisfied: google-cloud-storage>=1.16.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (1.16.1)
Requirement already satisfied: numpy in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from kfserver==0.1.0->alibiexplainer==0.1.0) (1.16.3)
Requirement already satisfied: scipy>=0.13.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from scikit-learn==0.20.3->alibiexplainer==0.1.0) (1.2.1)
Requirement already satisfied: pyyaml in /home/clive/.local/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (3.13)
Requirement already satisfied: redis in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (3.2.1)
Requirement already satisfied: Flask-OpenTracing==0.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (0.2.0)
Requirement already satisfied: grpcio-opentracing in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (1.1.4)
Requirement already satisfied: tensorflow in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (1.13.1)
Requirement already satisfied: flask in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (1.0.3)
Requirement already satisfied: jaeger-client==3.13.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (3.13.0)
Requirement already satisfied: grpcio in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (1.21.1)
Requirement already satisfied: opentracing<2,>=1.2.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (1.3.0)
Requirement already satisfied: flask-cors in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (3.0.8)
Requirement already satisfied: flatbuffers in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (1.11)
Requirement already satisfied: protobuf in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from seldon-core>=0.3->alibiexplainer==0.1.0) (3.8.0)
Requirement already satisfied: urllib3!=1.25.0,!=1.25.1,<1.26,>=1.21.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (1.25.3)
Requirement already satisfied: certifi>=2017.4.17 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (2019.3.9)
Requirement already satisfied: idna<2.9,>=2.5 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (2.8)
Requirement already satisfied: chardet<3.1.0,>=3.0.2 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from requests>=2.22.0->alibiexplainer==0.1.0) (3.0.4)
Requirement already satisfied: attrs>=17.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (19.1.0)
Requirement already satisfied: py>=1.5.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (1.8.0)
Requirement already satisfied: setuptools in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (41.0.1)
Requirement already satisfied: pluggy>=0.11 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (0.11.0)
Requirement already satisfied: more-itertools>=4.0.0; python_version > "2.7" in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (7.0.0)
Requirement already satisfied: atomicwrites>=1.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (1.3.0)
Requirement already satisfied: six>=1.10.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from pytest->alibiexplainer==0.1.0) (1.12.0)
Requirement already satisfied: typed-ast<1.4.0,>=1.3.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from mypy->alibiexplainer==0.1.0) (1.3.5)
Requirement already satisfied: mypy-extensions<0.5.0,>=0.4.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from mypy->alibiexplainer==0.1.0) (0.4.1)
Requirement already satisfied: python-dateutil in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from minio>=4.0.9->kfserver==0.1.0->alibiexplainer==0.1.0) (2.8.0)
Requirement already satisfied: pytz in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from minio>=4.0.9->kfserver==0.1.0->alibiexplainer==0.1.0) (2019.1)
Requirement already satisfied: google-cloud-core<2.0dev,>=1.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.0.1)
Requirement already satisfied: google-auth>=1.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.6.3)
Requirement already satisfied: google-resumable-media>=0.3.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (0.3.2)
Requirement already satisfied: keras-applications>=1.0.6 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (1.0.8)
Requirement already satisfied: keras-preprocessing>=1.0.5 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (1.1.0)
Requirement already satisfied: termcolor>=1.1.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (1.1.0)
Requirement already satisfied: wheel>=0.26 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (0.33.1)
Requirement already satisfied: astor>=0.6.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (0.8.0)
Requirement already satisfied: tensorboard<1.14.0,>=1.13.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (1.13.1)
Requirement already satisfied: absl-py>=0.1.6 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (0.7.1)
Requirement already satisfied: tensorflow-estimator<1.14.0rc0,>=1.13.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (1.13.0)
Requirement already satisfied: gast>=0.2.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (0.2.2)
Requirement already satisfied: Jinja2>=2.10 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from flask->seldon-core>=0.3->alibiexplainer==0.1.0) (2.10.1)
Requirement already satisfied: click>=5.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from flask->seldon-core>=0.3->alibiexplainer==0.1.0) (7.0)
Requirement already satisfied: Werkzeug>=0.14 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from flask->seldon-core>=0.3->alibiexplainer==0.1.0) (0.15.4)
Requirement already satisfied: itsdangerous>=0.24 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from flask->seldon-core>=0.3->alibiexplainer==0.1.0) (1.1.0)
Requirement already satisfied: threadloop<2,>=1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from jaeger-client==3.13.0->seldon-core>=0.3->alibiexplainer==0.1.0) (1.0.2)
Requirement already satisfied: thrift in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from jaeger-client==3.13.0->seldon-core>=0.3->alibiexplainer==0.1.0) (0.11.0)
Requirement already satisfied: google-api-core<2.0.0dev,>=1.11.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-cloud-core<2.0dev,>=1.0.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.11.1)
Requirement already satisfied: rsa>=3.1.4 in /home/clive/.local/lib/python3.7/site-packages (from google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (3.4.2)
Requirement already satisfied: cachetools>=2.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (3.1.1)
Requirement already satisfied: pyasn1-modules>=0.2.1 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (0.2.5)
Requirement already satisfied: h5py in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from keras-applications>=1.0.6->tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (2.9.0)
Requirement already satisfied: markdown>=2.6.8 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorboard<1.14.0,>=1.13.0->tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (3.1.1)
Requirement already satisfied: mock>=2.0.0 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from tensorflow-estimator<1.14.0rc0,>=1.13.0->tensorflow->seldon-core>=0.3->alibiexplainer==0.1.0) (3.0.5)
Requirement already satisfied: MarkupSafe>=0.23 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from Jinja2>=2.10->flask->seldon-core>=0.3->alibiexplainer==0.1.0) (1.1.1)
Requirement already satisfied: googleapis-common-protos!=1.5.4,<2.0dev,>=1.5.3 in /home/clive/anaconda3/envs/kfserving/lib/python3.7/site-packages (from google-api-core<2.0.0dev,>=1.11.0->google-cloud-core<2.0dev,>=1.0.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (1.6.0)
Requirement already satisfied: pyasn1>=0.1.3 in /home/clive/.local/lib/python3.7/site-packages (from rsa>=3.1.4->google-auth>=1.2.0->google-cloud-storage>=1.16.0->kfserver==0.1.0->alibiexplainer==0.1.0) (0.4.5)
Installing collected packages: alibiexplainer
  Found existing installation: alibiexplainer 0.1.0
      Uninstalling alibiexplainer-0.1.0:
            Successfully uninstalled alibiexplainer-0.1.0
	      Running setup.py develop for alibiexplainer
	      Successfully installed alibiexplainer
	      
```

To run static type checks:

```bash
mypy --ignore-missing-imports sklearnserver
```
An empty result will indicate success.


