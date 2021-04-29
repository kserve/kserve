FROM python:3.7-slim

COPY catboostserver catboostserver
COPY kfserving kfserving

RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./catboostserver
COPY third_party third_party

ENTRYPOINT ["python", "-m", "catboostserver"]
