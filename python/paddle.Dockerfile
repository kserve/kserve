FROM python:3.8

RUN pip install --upgrade pip

COPY kfserving kfserving
COPY paddleserver paddleserver

RUN pip install -e ./kfserving -e ./paddleserver

ENTRYPOINT ["python", "-m", "paddleserver"]
