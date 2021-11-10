FROM python:3.7

COPY artexplainer artexplainer
COPY kserve kserve

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./artexplainer
ENTRYPOINT ["python", "-m", "artserver"]
