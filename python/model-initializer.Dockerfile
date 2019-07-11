FROM python:3.7-slim

COPY ./kfserving ./kfserving
RUN pip install --upgrade pip && pip install -e ./kfserving

COPY ./model-initializer /model-initializer

RUN chmod +x /model-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work
ENTRYPOINT ["/model-initializer/scripts/initializer-entrypoint"]
