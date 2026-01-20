.PHONY: dev_install install_dependencies test type_check format clean

dev_install:
	uv sync --active --group test --group dev

install_dependencies:
	uv sync --active --group test

test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports aiserver

format:
	black aiserver/

clean:
	rm -rf __pycache__
	rm -rf .pytest_cache
	rm -rf .mypy_cache
	rm -rf *.egg-info
	find . -type d -name __pycache__ -exec rm -rf {} +
	find . -type f -name "*.pyc" -delete
