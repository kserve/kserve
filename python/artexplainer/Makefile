dev_install:
	uv sync --group test

test: type_check lint
	pytest -W ignore

lint:
	pylint --rcfile=../.pylintrc artexplainer
	pylint --rcfile=../.pylintrc tests

type_check:
	mypy --ignore-missing-imports artexplainer
