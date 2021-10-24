FROM python:3.7-slim

RUN apt-get update && apt-get install -y libglib2.0-0
COPY . .
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir kserve 
RUN pip install --no-cache-dir -e .
ENTRYPOINT ["python", "-m", "image_transformer"]
