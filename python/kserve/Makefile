.PHONY: test

dev_install:
	poetry version $(shell cat ../VERSION) && poetry install --with test,dev --extras "storage"

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
