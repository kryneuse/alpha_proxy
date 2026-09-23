"""gRPC server bootstrap for the PII detector service."""
from __future__ import annotations

import logging
from concurrent import futures

import grpc

from ..config import Config
from ..core.detector import PIIDetector
from .. import proto as pb
from .service import PIIDetectorService

logger = logging.getLogger("ml_service.server")


def build_server(config: Config) -> grpc.Server:
    """Build and return a configured gRPC server (not yet started)."""
    detector = PIIDetector(config)

    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=config.grpc_max_workers),
        options=[
            ("grpc.max_send_message_length", config.grpc_max_message_length),
            ("grpc.max_receive_message_length", config.grpc_max_message_length),
        ],
    )
    pb.add_PIIDetectorServicer_to_server(PIIDetectorService(detector), server)
    return server


def serve(config: Config) -> None:
    """Start the gRPC server and block until shutdown."""
    server = build_server(config)
    server.add_insecure_port(f"[::]:{config.grpc_port}")
    server.start()
    logger.info("PIIDetector gRPC server listening on port %d", config.grpc_port)
    server.wait_for_termination()


def main() -> None:
    logging.basicConfig(level=logging.INFO)
    config = Config.from_env()
    serve(config)


if __name__ == "__main__":
    main()