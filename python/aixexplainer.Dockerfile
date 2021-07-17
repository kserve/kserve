FROM python:3.7

COPY . .
RUN pip install --no-cache-dir --upgrade pip && pip install --no-cache-dir -e ./kfserving
RUN pip install --no-cache-dir -e ./aixexplainer
ENTRYPOINT ["python", "-m", "aixserver"]
