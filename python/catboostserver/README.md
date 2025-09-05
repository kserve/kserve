# CatBoost Server

[CatBoost](https://catboost.ai/) is a high-performance open source library for gradient boosting on decision trees.

To start the server locally for development needs, run the following command under this folder in your github repository.

```bash
python -m catboostserver --model_dir /path/to/model_dir --model_name catboost-model
```

## Development

Install the development dependencies with:

```bash
pip install -e .[test]
```

To run tests:

```bash
python -m pytest
```

To run static type checks:

```bash  
python -m mypy --ignore-missing-imports catboostserver
```

## Building your own CatBoost image

You can build and publish your own image for development needs. Please ensure that you modify the inferenceservice files for the `image` and add the `imagePullPolicy: Always`.

To build your own image, navigate up to the `python` directory and run:

```bash
docker build -t {username}/catboostserver -f catboost.Dockerfile .
```

To push your image:

```bash
docker push {username}/catboostserver
```