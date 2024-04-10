IMG ?= hypermode/huggingface-model-server:latest

dev_install:
	poetry install --with test

install_dependencies:
	poetry install --with=test --no-interaction --no-root
	
test: type_check
	pytest -W ignore

export_requirements:
	poetry export -f requirements.txt --output requirements.txt --without-hashes

build-docker:
	docker build -t $(IMG) --platform linux/amd64 .
