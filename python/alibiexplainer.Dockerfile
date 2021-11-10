FROM python:3.7

COPY alibiexplainer alibiexplainer
COPY kserve kserve
COPY third_party third_party

RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kserve
RUN pip install --no-cache-dir -e ./alibiexplainer
ENTRYPOINT ["python", "-m", "alibiexplainer"]
