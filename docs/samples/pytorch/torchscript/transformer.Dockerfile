FROM python:3.7-slim

RUN pip install --upgrade pip && pip install git+https://github.com/yuzisun/kfserving@torchscript&subdirectory=python/kfserving
COPY . .
RUN pip install -e .
ENTRYPOINT ["python", "-m", "image_transformer_v2"]
