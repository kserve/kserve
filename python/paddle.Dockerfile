FROM python:3.8

RUN pip install --upgrade pip

COPY kserve kserve
COPY paddleserver paddleserver

RUN pip install -e ./kserve -e ./paddleserver

ENTRYPOINT ["python", "-m", "paddleserver"]
