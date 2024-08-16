.PHONY: test

dev_install:
	# poetry install --with test --extras "storage ray"
	poetry install --with test --extras "ray"

install_dependencies:
	# poetry install --with=test --no-interaction --no-root --extras "storage ray"
	poetry install --with=test --no-interaction --no-root --extras "ray"

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
