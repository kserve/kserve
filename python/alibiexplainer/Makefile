
dev_install:
	pip install -e .[test]

test: type_check lint
	pytest -W ignore

lint:
	pylint --rcfile=../.pylintrc alibiexplainer

type_check:
	mypy --ignore-missing-imports alibiexplainer
