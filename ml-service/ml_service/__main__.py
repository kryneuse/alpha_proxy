"""Entry point: python -m ml_service."""
from .core import threading_setup  # noqa: F401  (must run first)
from .server.server import main

if __name__ == "__main__":
    main()