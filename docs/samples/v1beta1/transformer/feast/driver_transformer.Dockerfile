FROM python:3.8-slim

RUN apt-get update \
&& apt-get install -y --no-install-recommends git

COPY . .
RUN pip install pip==20.2
RUN pip install -e .
ENTRYPOINT ["python", "-m", "driver_transformer"]
