"""Text windowing for the two models.

LLAIM gate uses the author's sentence/line chunking (max 900 chars) plus a
token-level sliding window (max_length=384, stride=64) to handle chunks that
exceed the token limit.

RedMadRobot NER uses a token-level sliding window (max_length=512, stride=128).
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Iterator, List, Tuple

# Author's chunking boundaries for LLAIM.
_LINE_BREAKS = "\n\r\u2028\u2029"


@dataclass(frozen=True)
class Window:
    """A single tokenization window over a text span.

    char_start/char_end are character offsets into the original text.
    """

    char_start: int
    char_end: int
    # offset_mapping from the tokenizer: list of (start, end) char offsets
    # relative to the window text, one per token.
    offset_mapping: List[Tuple[int, int]]


def llaim_chunks(text: str, max_chars: int = 900) -> Iterator[Tuple[int, str]]:
    """Author's chunking: split long text at sentence/line boundaries.

    Mirrors LLAIMlegal/ru-legal-ner `_chunks`. Yields (start, chunk_text).
    """
    if len(text) <= max_chars:
        yield 0, text
        return
    start, n = 0, len(text)
    while start < n:
        end = min(start + max_chars, n)
        if end < n:
            b = max(
                text.rfind(". ", start, end),
                text.rfind("\n", start, end),
                text.rfind("; ", start, end),
            )
            if b > start:
                end = b + 1
        yield start, text[start:end]
        start = end


def sliding_windows(
    tokenizer,
    text: str,
    max_length: int,
    stride: int,
) -> Iterator[Window]:
    """Tokenize `text` with a sliding window and recover char offsets.

    Uses the tokenizer's offset_mapping to map tokens back to character
    positions. Windows overlap by `stride` tokens. Padding/special tokens are
    excluded from the returned offset_mapping.
    """
    enc = tokenizer(
        text,
        return_offsets_mapping=True,
        truncation=True,
        max_length=max_length,
        stride=stride,
        return_overflowing_tokens=True,
        return_tensors=None,
    )
    seq_ids = enc.sequence_ids(0) if hasattr(enc, "sequence_ids") else None
    for i, offset_mapping in enumerate(enc["offset_mapping"]):
        # Filter out special tokens (sequence_id is None) and padding.
        mapping = []
        for tok_idx, (s, e) in enumerate(offset_mapping):
            if s == e:
                continue
            if seq_ids is not None:
                sid = seq_ids[tok_idx] if isinstance(seq_ids, list) else None
                if sid is None:
                    continue
            mapping.append((s, e))
        if not mapping:
            continue
        yield Window(
            char_start=mapping[0][0],
            char_end=mapping[-1][1],
            offset_mapping=mapping,
        )


def llaim_windows(
    tokenizer,
    text: str,
    max_chars: int = 900,
    max_length: int = 384,
    stride: int = 64,
) -> Iterator[Tuple[int, Window]]:
    """Author chunking + token sliding window for the LLAIM gate.

    Yields (chunk_char_offset, Window) where chunk_char_offset is the offset of
    the chunk start within the original text.
    """
    for chunk_offset, chunk in llaim_chunks(text, max_chars):
        for win in sliding_windows(tokenizer, chunk, max_length, stride):
            yield chunk_offset, win


def rmr_windows(
    tokenizer,
    text: str,
    max_length: int = 512,
    stride: int = 128,
) -> Iterator[Window]:
    """Token sliding window for the RedMadRobot NER."""
    yield from sliding_windows(tokenizer, text, max_length, stride)