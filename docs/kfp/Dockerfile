FROM python:3.9-slim-bullseye
RUN apt-get update && apt-get install -y gcc python3-dev

COPY requirements.txt .
RUN python3 -m pip install -r \
    requirements.txt --quiet --no-cache-dir \
    && rm -f requirements.txt

ENV APP_HOME /app
COPY src $APP_HOME
WORKDIR $APP_HOME

ENTRYPOINT ["python"]
CMD ["kservedeployer.py"]
