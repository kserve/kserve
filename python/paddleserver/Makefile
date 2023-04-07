dev_install:
	poetry version $(shell cat ../VERSION) && poetry install --with test

test:
	pytest

type_check:
	mypy --ignore-missing-imports paddleserver