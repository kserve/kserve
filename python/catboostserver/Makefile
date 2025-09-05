dev_install:
	uv sync --group test

install_dependencies:
	uv sync --group test

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports catboostserver

fmt:
	python -m black . --config ../pyproject.toml

lint:
	python -m flake8 . --config=../.flake8
