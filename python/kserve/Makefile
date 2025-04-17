.PHONY: test

dev_install:
	uv sync --extra test --features "storage ray llm"

install_dependencies:
	uv sync --extra test --features "storage ray llm" --no-root

test:
	cd ../ && pytest -W ignore kserve/test

type_check:
	mypy --ignore-missing-imports kserve
