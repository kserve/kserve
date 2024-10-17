# Alibi Model Explainer

[Alibi](https://github.com/SeldonIO/alibi) server is an implementation of a KFServer for providing black box model explanation for KFServer models.

To start the server locally for development needs, run the following command under this folder in your github repository. 

```
make dev_install
```

After poetry has installed dependencies you should see:

```
	      Successfully installed alibiexplainer
```

You can check for successful installation by running the following command

```
$ python alibiexplainer/__main__.py {AnchorTabular|AnchorText|AnchorImages}  
...
2024-10-17 15:48:59.751 51916 kserve INFO [explainer.py:__init__():54] Predict URL set to None
```

## Samples

To run a local example follow the [income classifier explanation sample](../../docs/samples/explanation/alibi/income/README.md).

## Development

Install the development dependencies with:

```bash
make dev_install
```

The following indicates a successful install.

```
      Successfully installed alibiexplainer
	      
```

To run static type checks:

```bash
mypy --ignore-missing-imports alibiexplainer
```
An empty result will indicate success.


