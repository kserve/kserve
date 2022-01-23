FROM python:3.8

RUN pip install --upgrade pip

COPY kserve kserve
COPY paddleserver paddleserver
COPY third_party third_party

RUN pip install -e ./kserve -e ./paddleserver

USER 1000
ENTRYPOINT ["python", "-m", "paddleserver"]
