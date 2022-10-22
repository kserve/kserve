FROM python:3.7-slim

COPY third_party third_party

COPY kserve kserve
COPY VERSION VERSION

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve

RUN apt update && apt install -y build-essential
COPY aixexplainer aixexplainer
RUN pip install --no-cache-dir -e ./aixexplainer
RUN apt remove -y build-essential

RUN useradd kserve -m -u 1000 -d /home/kserve
USER 1000
ENTRYPOINT ["python", "-m", "aixserver"]
