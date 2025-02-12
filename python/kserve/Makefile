.PHONY: test

dev_install:
	poetry install --with test --extras "storage ray llm"

install_dependencies:
	poetry install --with=test --no-interaction --no-root --extras "storage ray llm"

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
