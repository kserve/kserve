dev_install:
	poetry install --with test

install_dependencies:
	poetry install --with=test --no-interaction --no-root

test:
	pytest

type_check:
	mypy --ignore-missing-imports paddleserver