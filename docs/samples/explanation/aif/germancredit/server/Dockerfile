# Use the official lightweight Python image.
# https://hub.docker.com/_/python
FROM python:3.10-slim

ENV APP_HOME /app
WORKDIR $APP_HOME

# Install production dependencies.
COPY requirements.txt ./
RUN pip install --no-cache-dir -r ./requirements.txt

# Copy local code to container image
COPY model.py ./

# Copy german-credit data
COPY german.data /usr/local/lib/python3.10/site-packages/aif360/data/raw/german/
COPY german.doc /usr/local/lib/python3.10/site-packages/aif360/data/raw/german/

CMD ["python", "model.py"]
# for local test use this CMD:
#CMD ["python", "model.py",
#     "-m", "aifserver",
#     "--predictor_host", "german-credit-predictor-default.default.svc.cluster.local",
#     "--model_name", "german-credit",
#     "--feature_names", "age", "sex", "credit_history=Delay", "credit_history=None/Paid", "credit_history=Other", "savings=500+", "savings=<500", "savings=Unknown/None", "employment=1-4 years", "employment=4+ years", "employment=unemployed",
#     "--label_names", "credit",
#     "--favorable_label", "1",
#     "--unfavorable_label", "2",
#     "--privileged_groups", "{\"age\": 1}",
#     "--unprivileged_groups", "{\"age\": 0}"]

# then python query_bias.py http://localhost:8080/v1/models/german-credit:predict input.json