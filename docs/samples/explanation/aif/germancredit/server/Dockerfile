# Use the official lightweight Python image.
# https://hub.docker.com/_/python
FROM python:3.7-slim

ENV APP_HOME /app
WORKDIR $APP_HOME

# Install production dependencies.
COPY requirements.txt ./
RUN pip install --no-cache-dir -r ./requirements.txt

# Copy local code to container image
COPY model.py ./

# Copy german-credit data
COPY german.data /usr/local/lib/python3.7/site-packages/aif360/data/raw/german/
COPY german.doc /usr/local/lib/python3.7/site-packages/aif360/data/raw/german/

CMD ["python", "model.py"]
