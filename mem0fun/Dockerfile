FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       curl \
       python3 \
       python3-pip \
       python3-venv \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:${PATH}"

COPY requirements.txt /tmp/requirements.txt
RUN python3 -m pip install --upgrade pip \
    && python3 -m pip install -r /tmp/requirements.txt

COPY demo.py /workspace/demo.py

CMD ["python3", "/workspace/demo.py"]
