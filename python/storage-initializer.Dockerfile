FROM python:3.7-slim

COPY ./kserve ./kserve
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir ./kserve

COPY ./storage-initializer /storage-initializer
COPY third_party third_party

RUN chmod +x /storage-initializer/scripts/initializer-entrypoint
RUN mkdir /work
WORKDIR /work

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["/storage-initializer/scripts/initializer-entrypoint"]
