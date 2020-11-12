FROM openjdk:11-slim

RUN apt update && apt install -y python3-minimal python3-pip && rm -rf /var/lib/apt/lists/*

COPY pmmlserver pmmlserver
COPY kfserving kfserving

RUN pip3 install --upgrade pip && pip3 install -e ./kfserving
RUN pip3 install -e ./pmmlserver
COPY third_party third_party

ENTRYPOINT ["python3", "-m", "pmmlserver"]
