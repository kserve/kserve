dev_install:
	poetry version $(shell cat ../VERSION) && poetry install --with test

test: type_check lint
	pytest -W ignore

lint:
	pylint --rcfile=../.pylintrc aixexplainer
	pylint --rcfile=../.pylintrc tests

type_check:
	mypy --ignore-missing-imports aixexplainer
