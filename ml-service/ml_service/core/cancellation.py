"""Cancellation helpers for cooperative deadline/context checking."""
from __future__ import annotations

from typing import Optional


class CancelledError(Exception):
    """Raised when the caller has cancelled the request."""


def check_cancelled(context) -> None:
    """Raise CancelledError if the gRPC context is cancelled or past deadline.

    `context` may be None (e.g. in tests or direct core usage), in which case
    the check is a no-op.
    """
    if context is None:
        return
    if context.is_active() is False:
        raise CancelledError("request cancelled")
    if context.time_remaining() is not None and context.time_remaining() <= 0:
        raise CancelledError("deadline exceeded")