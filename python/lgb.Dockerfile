FROM python:3.7-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
COPY third_party third_party
COPY kfserving kfserving
RUN pip install --upgrade pip && pip install -e ./kfserving

COPY lgbserver lgbserver
RUN pip install -e ./lgbserver

ENTRYPOINT ["python", "-m", "lgbserver"]
