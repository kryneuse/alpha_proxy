"""Integration tests for the gate and NER pipeline."""
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import pytest

from ml_service.config import Config
from ml_service.core.detector import PIIDetector


@pytest.fixture(scope="module")
def detector():
    return PIIDetector(Config())


def test_gate_closed_no_pii(detector):
    text = "Сегодня хорошая погода. Мы гуляли в парке и обсуждали планы."
    res = detector.detect_chunk("c1", text)
    assert res.audit.gate_open is False
    assert res.audit.gate_score < 0.001
    assert res.entities == []


def test_gate_open_with_phone(detector):
    text = "Позвоните мне на +7 999 123-45-67"
    res = detector.detect_chunk("c1", text)
    assert res.audit.gate_open is True
    assert "PHONE" in res.audit.matched_types
    phones = [e for e in res.entities if e.type == "PHONE"]
    assert phones
    assert text[phones[0].start : phones[0].end] == "+7 999 123-45-67"


def test_full_name_normalization(detector):
    text = "Меня зовут Иванов Пётр Сергеевич"
    res = detector.detect_chunk("c1", text)
    names = [e for e in res.entities if e.type == "FULL_NAME"]
    assert names
    assert text[names[0].start : names[0].end] == "Иванов Пётр Сергеевич"


def test_email_detection(detector):
    text = "Напишите на ivanov@mail.ru"
    res = detector.detect_chunk("c1", text)
    emails = [e for e in res.entities if e.type == "EMAIL"]
    assert emails
    assert text[emails[0].start : emails[0].end] == "ivanov@mail.ru"


def test_offsets_are_unicode_code_points(detector):
    # Cyrillic chars are 1 code point each; verify offsets align with slicing.
    text = "Иванов Пётр, тел. +7 999 123-45-67"
    res = detector.detect_chunk("c1", text)
    for e in res.entities:
        assert 0 <= e.start < e.end <= len(text)
        assert text[e.start : e.end] != ""


def test_ignored_types_do_not_open_gate(detector):
    # ORG is an ignored type and must not open the gate by itself.
    text = "ООО «Ромашка» занимается продажами"
    res = detector.detect_chunk("c1", text)
    # ORG may be detected but must not be in matched_types.
    assert "ORG" not in res.audit.matched_types
    assert "ORG" in res.audit.ignored_types or res.audit.gate_open is False


def test_country_maps_to_address(detector):
    # COUNTRY has no proto enum value; per product decision it is emitted as
    # ADDRESS, while CITY/STREET/HOUSE stay separate entities.
    text = "Россия, г. Москва, ул. Тверская, д. 10"
    res = detector.detect_chunk("c1", text)
    types = [e.type for e in res.entities]
    assert "ADDRESS" in types
    assert "CITY" in types
    assert "STREET" in types
    assert "HOUSE" in types
    # Address components are not merged into one entity.
    addresses = [e for e in res.entities if e.type == "ADDRESS"]
    assert any(text[e.start : e.end] == "Россия" for e in addresses)


def test_long_text_with_pii_at_end(detector):
    # PII at the end of a long text must be detected (sliding windows + chunk
    # offset recovery).
    text = ("Это длинный текст без персональных данных. " * 50) + "Иванов Пётр, тел. +7 999 123-45-67"
    res = detector.detect_chunk("c1", text)
    assert res.audit.gate_open is True
    types = [e.type for e in res.entities]
    assert "FULL_NAME" in types
    assert "PHONE" in types
    # Offsets are relative to the full chunk text.
    for e in res.entities:
        assert 0 <= e.start < e.end <= len(text)