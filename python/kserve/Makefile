.PHONY: test

dev_install:
	pip install -e .
	pip install -e .[test]

test:
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports kserve
