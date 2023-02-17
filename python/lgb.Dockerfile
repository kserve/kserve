FROM python:3.9-slim-bullseye

COPY third_party third_party

COPY kserve kserve
COPY VERSION VERSION
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

COPY storage-initializer storage-initializer
RUN pip install --no-cache-dir -e ./storage-initializer

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY lgbserver lgbserver
RUN pip install --no-cache-dir -e ./lgbserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "lgbserver"]
