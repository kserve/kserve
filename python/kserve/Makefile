.PHONY: test

dev_install:
	pip install -e .
	pip install -e .[test]

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
