FROM python:3.7-slim

RUN apt-get update \
&& apt-get install -y --no-install-recommends git

COPY . .
RUN pip install --upgrade pip
RUN pip install kfserving==0.5.0rc0
RUN pip install -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
