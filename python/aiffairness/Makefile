dev_install:
	poetry install --with test

test: type_check lint
	pytest -W ignore

lint:
	pylint --rcfile=../.pylintrc aiffairness
	pylint --rcfile=../.pylintrc tests

type_check:
	mypy --ignore-missing-imports aiffairness
