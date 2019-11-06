# Alibi Model Explainer

[Alibi](https://github.com/SeldonIO/alibi) server is an implementation of a KFServer for providing black box model explanation for KFServer models.

To start the server locally for development needs, run the following command under this folder in your github repository. 

```
pip install -e .
```

After pip has installed dependencies you should see:

```
	      Successfully installed alibiexplainer
```

You can check for successful installation by running the following command

```
usage: __main__.py [-h] [--http_port HTTP_PORT] [--grpc_port GRPC_PORT]
                   [--predict_url PREDICT_URL] [--method {ExplainerMethod.anchor_tabular}]
__main__.py: error: the following arguments are required: --predict_url
```

## Samples

To run a local example follow the [income classifier explanation sample](../../docs/samples/explanation/alibi/income/README.md).

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

The following indicates a successful install.

```
      Successfully installed alibiexplainer
	      
```

To run static type checks:

```bash
mypy --ignore-missing-imports sklearnserver
```
An empty result will indicate success.


