"""Core data types for the PII detection pipeline."""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Optional


@dataclass(frozen=True)
class Entity:
    """A detected PII entity with character offsets in the chunk text.

    start/end are Unicode code point indices into the original Python string,
    half-open interval [start, end).
    """

    type: str
    start: int
    end: int
    score: float


@dataclass
class ChunkAudit:
    """Per-chunk audit record produced during detection."""

    chunk_id: str
    backend: str = ""
    route_reason: str = ""
    queue_ms: float = 0.0
    gate_ran: bool = False
    gate_open: bool = False
    gate_score: float = 0.0
    gate_ms: float = 0.0
    ner_ms: float = 0.0
    matched_types: List[str] = field(default_factory=list)
    ignored_types: List[str] = field(default_factory=list)
    error: Optional[str] = None
    gate_error: Optional[str] = None
    gate_error_fallback: bool = False


@dataclass
class ChunkResult:
    """Result of processing a single chunk."""

    chunk_id: str
    entities: List[Entity] = field(default_factory=list)
    error_code: str = "NONE"
    audit: Optional[ChunkAudit] = None


@dataclass
class BatchResult:
    """Result of processing a whole batch."""

    batch_id: str
    model_version: str
    offset_unit: str
    results: List[ChunkResult] = field(default_factory=list)