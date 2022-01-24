FROM python:3.8

RUN pip install --upgrade pip

COPY kserve kserve
COPY paddleserver paddleserver
COPY third_party third_party

RUN pip install -e ./kserve -e ./paddleserver

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "paddleserver"]
