FROM python:3.7-slim

COPY ./kfserving ./kfserving
RUN pip install --upgrade pip && pip install ./kfserving

COPY ./storage-initializer /storage-initializer
COPY third_party third_party

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
