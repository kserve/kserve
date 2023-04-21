FROM pytorch/pytorch:2.0.0-cuda11.7-cudnn8-runtime

COPY requirements.txt ./requirements.txt
RUN pip3 install --no-cache-dir -r requirements.txt

COPY models.py /app/models.py
ENV PYTHONPATH=/app/
WORKDIR /app

ENV MODEL_URL="/mnt/models" \
    LOAD_IN_8BIT="False" \
    GPU_ENABLED="False" \
    MODEL_NAME="bloom"

ENV LD_LIBRARY_PATH="${LD_LIBRARY_PATH}:/usr/local/cuda-11.7/targets/x86_64-linux/lib"

CMD python models.py --model_name $MODEL_NAME