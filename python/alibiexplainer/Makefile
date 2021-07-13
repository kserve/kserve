
dev_install:
	pip install -e .[test]

# Requires sklearnserver to be installed
test: type_check lint
	pytest -W ignore

lint:
	pylint --rcfile=../.pylintrc alibiexplainer
	pylint --rcfile=../.pylintrc tests

type_check:
	mypy --ignore-missing-imports alibiexplainer
