"""Concurrent adaptive inference preserves IDs, spans and pool bounds."""
from concurrent.futures import ThreadPoolExecutor
from dataclasses import replace
from pathlib import Path

import pytest

from ml_service.config import Config
from ml_service.core.detector import PIIDetector


def test_concurrent_real_models():
    cfg = replace(Config(), quality_queue=0, spacy_queue=128)
    if not (Path(cfg.ner.dir)/'model.int8.onnx').exists():
        pytest.skip('Quality artifacts not installed')
    detector = PIIDetector(cfg)
    try:
        def run(i):
            text = f'Иванов Пётр Сергеевич, email sample{i}@example.com.'
            result = detector.detect_chunk_v2(str(i),text,text)
            assert result.chunk_id == str(i)
            assert result.error_code == 'NONE'
            assert result.entities
            assert all(0 <= e.start < e.end <= len(text) for e in result.entities)
            return result.audit.backend
        with ThreadPoolExecutor(max_workers=12) as pool:
            routes = list(pool.map(run, range(48)))
        assert 'quality' in routes and 'spacy_sm' in routes
        stats = detector.scheduler.snapshot()
        assert stats['pending'] == {'quality':0,'spacy_sm':0}
        assert sum(stats['counters'].get('completed_'+r,0) for r in ('quality','spacy_sm')) == 48
    finally:
        detector.close()
