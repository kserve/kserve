dev_install:
	poetry install --with test --no-interaction

install_dependencies:
	poetry install --with=test --no-interaction --no-root
	
test: type_check
	pytest -W ignore

type_check:
	mypy --ignore-missing-imports huggingfaceserver 
