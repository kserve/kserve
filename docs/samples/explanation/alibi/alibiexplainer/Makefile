
dev_install:
	uv sync --group test
	uv pip install --no-deps -e ../../../../../python/sklearnserver

install_dependencies:
	uv sync --group test --no-install-project
	uv pip install --no-deps -e ../../../../../python/sklearnserver

test: type_check lint
	uv pip install --no-deps -e ../../../../../python/sklearnserver
	uv run pytest -W ignore

lint:
	pylint --rcfile=../.pylintrc alibiexplainer
	pylint --rcfile=../.pylintrc tests

type_check:
	uv run mypy --ignore-missing-imports alibiexplainer
