"""V2 integration parity against the same loaded model artifacts."""
import json
from dataclasses import replace
from pathlib import Path

import pytest

from ml_service.config import Config
from ml_service.core.detector import PIIDetector
from ml_service.core.v14_repair import repair_structured_v3


@pytest.fixture(scope="module")
def quality():
    cfg = replace(Config(), backend="quality")
    if not (Path(cfg.ner.dir) / "model.int8.onnx").exists():
        pytest.skip("Quality artifacts not installed")
    detector = PIIDetector(cfg)
    yield detector
    detector.close()


@pytest.mark.parametrize("text", [
    "Сегодня хорошая погода. Обсуждаем проект.",
    "Иванов Пётр Сергеевич, тел. +7 999 123-45-67, email example@example.com",
    "👩‍💻 Россия, г. Москва, ул. Тверская, дом 10, квартира 7.",
    "Дата рождения 12.03.1991, паспорт выдан 20.04.2011, код 770-001.",
])
def test_quality_matches_gate_and_ner_reference(quality, text):
    score = quality._gate.score(text)
    result = quality.detect_chunk_v2("c", text, text)
    assert result.error_code == "NONE"
    assert result.audit.backend == "quality"
    assert result.audit.gate_score == score
    expected = repair_structured_v3(text, quality._ner.run(text)) if score >= quality._config.gate_threshold else []
    assert {(e.type,e.start,e.end) for e in result.entities} == {(e['type'],e['start'],e['end']) for e in expected}


def test_quality_blank_residual_preserves_existing_bypass(quality):
    result = quality.detect_chunk_v2("c", "Иван", " " * len("Иван".encode()))
    assert not result.audit.gate_ran
    assert result.entities == []


def test_spacy_artifact_matches_transfer_fixtures():
    cfg = replace(Config(), backend="spacy_sm")
    if not (Path(cfg.spacy_dir) / "pii_multibio/model.bin").exists():
        pytest.skip("spaCy artifact not installed")
    fixture = Path(__file__).parent / "fixtures/spacy_transfer"
    rows = [json.loads(line) for line in (fixture/'input.jsonl').read_text().split('\n') if line]
    expected = [json.loads(line) for line in (fixture/'expected_repaired.jsonl').read_text().split('\n') if line]
    detector = PIIDetector(cfg)
    try:
        for row, ref in zip(rows, expected, strict=True):
            result = detector.detect_chunk_v2(row['id'], row['text'], ' '*len(row['text'].encode()))
            assert result.error_code == "NONE"
            assert {(e.type,e.start,e.end) for e in result.entities} == {(e['type'],e['start'],e['end']) for e in ref['entities']}
    finally:
        detector.close()
