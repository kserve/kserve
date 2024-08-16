.PHONY: test

dev_install:
	poetry install --with test --extras "ray llm"

install_dependencies:
	poetry install --with=test --no-interaction --no-root --extras "ray llm"

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve