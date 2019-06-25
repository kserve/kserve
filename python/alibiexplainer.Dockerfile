FROM python:3.7-slim

RUN apt update && apt-get install -y libglib2.0-0 libsm6 libxext6 libxrender1
COPY . .
RUN pip install --upgrade pip && pip install -e ./kfserving
RUN pip install -e ./alibiexplainer
ENTRYPOINT ["python", "-m", "alibiexplainer"]
