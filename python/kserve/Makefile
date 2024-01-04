.PHONY: test

dev_install:
	poetry install --with test --extras "storage"

install_dependencies:
	poetry install --with=test --no-interaction --no-root --extras "storage"

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
