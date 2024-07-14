FROM python:3.9-slim

COPY cache_inference cache_inference
WORKDIR cache_inference
RUN pip install --upgrade pip
RUN pip install -e .
ENTRYPOINT ["python", "-m", "cache_inference"]
