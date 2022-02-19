FROM python:3.7-slim

COPY sklearnserver sklearnserver
COPY kserve kserve
COPY third_party third_party

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./sklearnserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "sklearnserver"]
