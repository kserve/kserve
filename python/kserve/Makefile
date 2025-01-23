.PHONY: test

dev_install:
	poetry install --with test --extras "ray"
	cd ../storage && poetry install

install_dependencies:
	poetry install --with test --no-interaction --no-root --extras "ray"
	cd ../storage && poetry install --no-interaction --no-root

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
