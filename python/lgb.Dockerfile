FROM python:3.7-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
COPY third_party third_party
COPY kserve kserve
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

COPY lgbserver lgbserver
RUN pip install --no-cache-dir -e ./lgbserver

ENTRYPOINT ["python", "-m", "lgbserver"]
