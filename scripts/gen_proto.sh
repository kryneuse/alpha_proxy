#!/usr/bin/env bash
# Regenerate Go and Python protobuf/gRPC code from the single source of truth
# api/ml/v1/pii.proto. Run from the repository root.
#
# Required tools (versions pinned to match the checked-in generated code):
#   - protoc            v3.21.12
#   - protoc-gen-go     v1.36.9   (go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.9)
#   - protoc-gen-go-grpc v1.5.1   (go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1)
#   - grpcio-tools      1.80.0    (pip install grpcio-tools==1.80.0)
#
# The Python modules are generated from the same pii.proto and named pii_pb2 /
# pii_pb2_grpc (not the old pii_detector_*). Update ml_service/proto/__init__.py
# and imports accordingly.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PROTO_SRC="api/ml/v1/pii.proto"
GO_OUT="gen"
PY_OUT="ml-service/ml_service/proto"

# --- Go ---
protoc \
  -I api \
  --go_out="$GO_OUT" --go_opt=paths=source_relative \
  --go-grpc_out="$GO_OUT" --go-grpc_opt=paths=source_relative \
  "$PROTO_SRC"

# --- Python ---
python3 -m grpc_tools.protoc \
  -I api/ml/v1 \
  --python_out="$PY_OUT" \
  --grpc_python_out="$PY_OUT" \
  "$PROTO_SRC"

echo "Generated Go code in $GO_OUT/ml/v1 and Python code in $PY_OUT"