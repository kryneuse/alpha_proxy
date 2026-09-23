"""Text windowing for the two models.

LLAIM gate uses the author's sentence/line chunking (max 900 chars) plus a
token-level sliding window (max_length=384, stride=64) to handle chunks that
exceed the token limit.

RedMadRobot NER uses a token-level sliding window (max_length=512, stride=128).

Each Window carries the ready model inputs (input_ids, attention_mask,
token_type_ids) together with the char offset_mapping, so the caller does not
re-tokenize the window before inference.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Dict, Iterator, List, Tuple

# Author's chunking boundaries for LLAIM.
_LINE_BREAKS = "\n\r\u2028\u2029"


@dataclass(frozen=True)
class Window:
    """A single tokenization window over a text span.

    char_start/char_end are character offsets into the original text.
    inputs holds the ready model inputs (numpy arrays) for this window.
    offset_mapping is the list of (start, end) char offsets relative to the
    window text, one per token (special/padding tokens excluded).
    """

    char_start: int
    char_end: int
    inputs: Dict[str, object]
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


def _to_numpy(enc: Dict) -> Dict[str, object]:
    """Convert a tokenizer encoding to numpy arrays for ORT feeds."""
    out = {}
    for key, val in enc.items():
        if key == "offset_mapping":
            continue
        if hasattr(val, "numpy"):
            out[key] = val.numpy()
        else:
            out[key] = val
    return out


def sliding_windows(
    tokenizer,
    text: str,
    max_length: int,
    stride: int,
) -> Iterator[Window]:
    """Tokenize `text` with a sliding window and recover char offsets.

    Uses the tokenizer's offset_mapping to map tokens back to character
    positions. Windows overlap by `stride` tokens. Each Window carries the
    ready model inputs so the caller does not re-tokenize.
    """
    enc = tokenizer(
        text,
        return_offsets_mapping=True,
        truncation=True,
        max_length=max_length,
        stride=stride,
        return_overflowing_tokens=True,
        return_tensors="np",
    )
    for i, offset_mapping in enumerate(enc["offset_mapping"]):
        # Keep the full offset_mapping (including special/padding tokens) so it
        # stays aligned with the logits rows. char_start/char_end are derived
        # from the first/last real token.
        real = [(int(s), int(e)) for s, e in offset_mapping if int(s) != int(e)]
        if not real:
            continue
        # Build per-window model inputs (keep the batch dimension).
        inputs = {}
        for key in ("input_ids", "attention_mask", "token_type_ids"):
            if key in enc:
                inputs[key] = enc[key][i : i + 1]
        yield Window(
            char_start=real[0][0],
            char_end=real[-1][1],
            inputs=inputs,
            offset_mapping=[(int(s), int(e)) for s, e in offset_mapping],
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