"""Tests for text windowing."""
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from ml_service.core.windowing import llaim_chunks, sliding_windows


def test_llaim_chunks_short_text():
    text = "Короткий текст."
    chunks = list(llaim_chunks(text, max_chars=900))
    assert chunks == [(0, text)]


def test_llaim_chunks_long_text():
    text = "Предложение первое. " * 100  # > 900 chars
    chunks = list(llaim_chunks(text, max_chars=900))
    assert len(chunks) > 1
    # Chunks are contiguous and cover the whole text.
    assert chunks[0][0] == 0
    for i in range(1, len(chunks)):
        assert chunks[i][0] == chunks[i - 1][0] + len(chunks[i - 1][1])
    total = sum(len(c) for _, c in chunks)
    assert total == len(text)


def test_llaim_chunks_boundary():
    # Splits at ". " boundary; the chunk includes the period but not the space.
    text = "A" * 500 + ". " + "B" * 500
    chunks = list(llaim_chunks(text, max_chars=900))
    assert len(chunks) == 2
    assert chunks[0][1].endswith(".")
    # Second chunk starts at the space after the period.
    assert chunks[1][0] == 501
    assert chunks[1][1] == " " + "B" * 500


def test_sliding_windows_offsets():
    from transformers import AutoTokenizer

    tokenizer = AutoTokenizer.from_pretrained(
        os.path.join(os.path.dirname(__file__), "..", "models", "llaim-ru-legal-ner")
    )
    text = "Иванов Пётр Сергеевич, паспорт 45 11 123456"
    windows = list(sliding_windows(tokenizer, text, max_length=384, stride=64))
    assert len(windows) == 1
    win = windows[0]
    assert win.char_start == 0
    assert win.char_end == len(text)
    # Offsets map back to the original text.
    for s, e in win.offset_mapping:
        assert text[s:e] != ""


def test_sliding_windows_long_text():
    from transformers import AutoTokenizer

    tokenizer = AutoTokenizer.from_pretrained(
        os.path.join(os.path.dirname(__file__), "..", "models", "llaim-ru-legal-ner")
    )
    text = "слово " * 500  # long enough to overflow 384 tokens
    windows = list(sliding_windows(tokenizer, text, max_length=384, stride=64))
    assert len(windows) > 1
    # Windows overlap.
    assert windows[1].char_start < windows[0].char_end