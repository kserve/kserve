FROM python:3.7-slim

RUN apt-get update && apt-get install -y libglib2.0-0
COPY . .
RUN pip install --upgrade pip && pip install kfserving
RUN pip install -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
