FROM python:3.7-slim

COPY ./kfserving ./kfserving
RUN pip install --upgrade pip && pip install -e ./kfserving

COPY ./commands /commands
RUN chmod +x /commands/download
RUN mkdir /work
WORKDIR /work
ENTRYPOINT ["/commands/download"]
