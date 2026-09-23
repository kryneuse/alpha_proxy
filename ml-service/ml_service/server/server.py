"""gRPC server bootstrap for the PII detector service (V2)."""
from __future__ import annotations

import logging
import signal
from concurrent import futures

import grpc

from ..config import Config
from ..core.detector import PIIDetector
from .. import proto as pb
from .service import PIIDetectorService

logger = logging.getLogger("ml_service.server")


def build_server(config: Config) -> grpc.Server:
    """Build and return a configured gRPC server (not yet started).

    Constructing PIIDetector loads and warms both graphs; a missing or
    incompatible model raises here, so the service never starts partially.
    """
    detector = PIIDetector(config)

    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=config.grpc_max_workers),
        maximum_concurrent_rpcs=config.grpc_max_concurrent_rpcs,
        options=[
            ("grpc.max_send_message_length", config.grpc_max_message_length),
            ("grpc.max_receive_message_length", config.grpc_max_message_length),
        ],
    )
    pb.add_PIIDetectorServicer_to_server(PIIDetectorService(detector, config), server)
    server.pii_detector = detector
    return server


def serve(config: Config) -> None:
    """Start the gRPC server and block until shutdown."""
    server = build_server(config)
    from .metrics import start_metrics
    metrics = None
    try:
        if not server.add_insecure_port(f"{config.grpc_host}:{config.grpc_port}"):
            raise RuntimeError("Unable to bind ML gRPC port")
        if config.metrics_port:
            metrics = start_metrics(server.pii_detector.scheduler, config.metrics_host, config.metrics_port)
        server.start()
        signal.signal(signal.SIGTERM, lambda *_: server.stop(3))
        logger.info("PIIDetector listening on port %d; backend=%s; quality_workers=%d; spacy_workers=%d",
                    config.grpc_port, config.backend, config.quality_workers, config.spacy_workers)
        server.wait_for_termination()
    except KeyboardInterrupt:
        pass
    finally:
        server.stop(3).wait()
        if metrics:
            metrics.shutdown()
            metrics.server_close()
        server.pii_detector.close()


def main() -> None:
    logging.basicConfig(level=logging.INFO)
    config = Config.from_env()
    serve(config)


if __name__ == "__main__":
    main()