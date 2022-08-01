FROM python:3.9-slim-bullseye

ARG DEBIAN_FRONTEND=noninteractive

COPY third_party third_party

COPY kserve kserve
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

RUN apt-get update && apt-get install -y \
    gcc \
    libkrb5-dev \
    krb5-config \
 && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir krbcontext==0.10 hdfs~=2.6.0 requests-kerberos==0.14.0

COPY ./storage-initializer /storage-initializer

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
