FROM python:3.7-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY kserve kserve
COPY lgbserver lgbserver
COPY third_party third_party

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./lgbserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "lgbserver"]
