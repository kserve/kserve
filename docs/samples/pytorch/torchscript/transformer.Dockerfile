FROM python:3.7-slim
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y git
RUN pip install --upgrade pip && pip install git+https://github.com/kubeflow/kfserving@torchscript#subdirectory=python/kfserving
COPY . .
RUN pip install -e .
ENTRYPOINT ["python", "-m", "image_transformer_v2"]
