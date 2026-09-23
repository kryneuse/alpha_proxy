# syntax=docker/dockerfile:1

# Runtime stage for the Python ML PII detection gRPC service.
# Models are NOT baked into the image; they are mounted at /models via a volume.
FROM python:3.12-slim-bookworm

# Install only the system runtime packages required by onnxruntime.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libgomp1 \
    && rm -rf /var/lib/apt/lists/*

# Install Python runtime dependencies first to leverage layer caching.
COPY requirements.txt requirements-spacy.txt ./
RUN python -m pip install --no-cache-dir --disable-pip-version-check -r requirements.txt

# Copy only the service package.
COPY ml_service ./ml_service

# Create an unprivileged system user/group without shell or home directory.
RUN groupadd --system --gid 10002 alpha-ml \
    && useradd --system --uid 10002 --gid 10002 --no-create-home --shell /usr/sbin/nologin alpha-ml

USER 10002:10002

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONHASHSEED=random \
    ML_MODELS_DIR=/models \
    ML_GRPC_HOST=0.0.0.0 \
    ML_GRPC_PORT=50051 \
    ML_METRICS_HOST=0.0.0.0 \
    ML_METRICS_PORT=9091 \
    HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1

EXPOSE 50051 9091

STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD python -c "import os, socket; port=int(os.environ.get('ML_GRPC_PORT', '50051')); s=socket.create_connection(('127.0.0.1', port), timeout=5); s.close()"

ENTRYPOINT ["python", "-m", "ml_service"]