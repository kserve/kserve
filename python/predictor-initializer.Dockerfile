FROM python:3.7-slim

COPY ./kfserving ./kfserving
RUN pip install --upgrade pip && pip install -e ./kfserving

COPY ./predictor-initializer /predictor-initializer

RUN chmod +x /predictor-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work
ENTRYPOINT ["/predictor-initializer/scripts/initializer-entrypoint"]
