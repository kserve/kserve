FROM python:3.7-slim

COPY custom_model custom_model 
COPY kserve kserve

RUN pip install --upgrade pip && pip install -e ./kserve
RUN pip install -e ./custom_model
COPY third_party third_party

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "custom_model"]
