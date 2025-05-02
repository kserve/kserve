.PHONY: test

dev_install:
	uv sync --active --group test --extra storage --extra ray --extra llm

install_dependencies:
	uv sync --active --group test --extra storage --extra ray --extra llm

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
