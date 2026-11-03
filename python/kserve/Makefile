.PHONY: test

dev_install:
	uv sync --active --group test --extra ray --extra llm
	uv pip install '../storage[confidential]' --no-cache

install_dependencies:
	uv sync --active --group test --extra ray --extra llm
	uv pip install '../storage[confidential]' --no-cache

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
