.PHONY: test

dev_install:
	poetry install --with test,dev --extras "storage ray-serve"

install_dependencies:
	poetry install --with=test --no-interaction --no-root --extras "storage ray-serve"

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
