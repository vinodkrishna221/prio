# =====================================================================
# Stage 1: Build & Compile Protobufs
# =====================================================================
FROM python:3.12-slim AS builder

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install protobuf tools
RUN pip install --no-cache-dir grpcio-tools==1.60.0 "setuptools<82"

# Copy proto directory and agent codebase
COPY proto/ /app/proto/
COPY services/python-agent/ /app/services/python-agent/

# Create protos output directory if missing
RUN mkdir -p /app/services/python-agent/protos

# Compile Protobuf schemas
RUN python -m grpc_tools.protoc -I /app/proto --python_out=/app/services/python-agent/protos --grpc_python_out=/app/services/python-agent/protos /app/proto/triage.proto && \
    python -m grpc_tools.protoc -I /app/proto --python_out=/app/services/python-agent/protos --grpc_python_out=/app/services/python-agent/protos /app/proto/scheduler.proto && \
    python -m grpc_tools.protoc -I /app/proto --python_out=/app/services/python-agent/protos --grpc_python_out=/app/services/python-agent/protos /app/proto/genome.proto

# =====================================================================
# Stage 2: Runtime Environment
# =====================================================================
FROM python:3.12-slim

WORKDIR /app/services/python-agent

# Install dependencies from requirements.txt
COPY services/python-agent/requirements.txt /app/services/python-agent/requirements.txt
RUN pip install --no-cache-dir -r /app/services/python-agent/requirements.txt

# Copy codebase and compiled stubs from builder
COPY services/python-agent/ /app/services/python-agent/
COPY --from=builder /app/services/python-agent/protos/ /app/services/python-agent/protos/

# Expose gRPC port
EXPOSE 50051

ENV PORT=50051
ENV PYTHONPATH=/app/services/python-agent

# Start service
CMD ["python", "main.py"]
