FROM python:3.7-slim

COPY custom_model custom_model 
COPY kfserving kfserving

RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./custom_model
COPY third_party third_party

ENTRYPOINT ["python", "-m", "custom_model"]
