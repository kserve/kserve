# Alibi Model Explainer

[Alibi](https://github.com/SeldonIO/alibi) server is an implementation of a KFServer for providing black box model explanation for KFServer models.

To start the server locally for development needs, run the following command under this folder in your github repository. 

```
make dev_install
```

After uv has installed dependencies you should see the environment synced with `alibiexplainer` and its dependencies.

You can check for successful installation by running the following command

```
$ uv run python alibiexplainer/__main__.py {AnchorTabular|AnchorText|AnchorImages}  
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

A successful `uv sync` finishes without errors and leaves a `.venv` with the project installed.

To run static type checks:

```bash
uv run mypy --ignore-missing-imports alibiexplainer
```
An empty result will indicate success.

