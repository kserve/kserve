
dev_install:
	uv sync --active --group test

install_dependencies:
	uv sync --active --group test
	
test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports sklearnserver