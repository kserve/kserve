FROM python:3.7-slim

COPY third_party third_party

COPY kserve kserve
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

COPY custom_model custom_model
RUN pip install -r ./custom_model/requirements.txt 

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "custom_model"]
