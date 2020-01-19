FROM python:3.7-slim

COPY sklearnserver sklearnserver
COPY kfserving kfserving

RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./sklearnserver
ENTRYPOINT ["python", "-m", "sklearnserver"]
