dev_install:
	poetry install --with test

test: type_check lint
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports custom_model
