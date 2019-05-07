## Development

Use conda to create a new environment

```
conda create --name kfserving python=3.7
conda activate kfserving
```

Install package for development

```
pip install -e .
```

To install development requirements

```
pip install -r dev_requirements.txt
```

To run tests:

```
make test
```
