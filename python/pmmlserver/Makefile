
dev_install:
	poetry version $(shell cat ../VERSION) &&  poetry install --with test

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports pmmlserver