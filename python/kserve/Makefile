.PHONY: test

dev_install:
	poetry install --with test

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
