.PHONY: test

dev_install:
	uv sync --active --group test --extra ray --extra llm
	cd ../../storage && uv sync --active --no-cache

install_dependencies:
	uv sync --active --group test --extra ray --extra llm
	cd ../../storage && uv sync --active --no-cache

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
