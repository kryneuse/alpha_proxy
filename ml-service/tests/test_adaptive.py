"""Deterministic admission, isolation and cancellation tests, without model files."""
import threading
from concurrent.futures import ThreadPoolExecutor
from dataclasses import replace

import pytest

from ml_service.config import Config
from ml_service.core.adaptive import AdaptiveScheduler, Overloaded, RequestCancelled
from ml_service.core.detector import PIIDetector
from ml_service.core.types import ChunkResult


def config(**kwargs):
    return replace(Config(), quality_workers=1, spacy_workers=1,
                   quality_queue=0, spacy_queue=0, **kwargs)


def test_fast_pool_runs_while_quality_is_blocked_and_recovers_after_hold():
    clock = [10.0]
    scheduler = AdaptiveScheduler(config(), clock=lambda: clock[0])
    entered, release = threading.Event(), threading.Event()

    def slow(route, reason, wait):
        assert route == "quality"
        entered.set()
        assert release.wait(2)
        return ChunkResult("quality")

    try:
        first = scheduler.submit(slow, 350)
        assert entered.wait(1)
        fast = scheduler.submit(lambda route, reason, wait: ChunkResult(route), 350)
        assert fast.result(timeout=1).chunk_id == "spacy_sm"
        assert not first.done()
        release.set()
        first.result(timeout=1)
        held = scheduler.submit(lambda route, reason, wait: ChunkResult(route), 350)
        assert held.result(timeout=1).chunk_id == "spacy_sm"
        clock[0] += 1
        recovered = scheduler.submit(lambda route, reason, wait: ChunkResult(route), 350)
        assert recovered.result(timeout=1).chunk_id == "quality"
        assert scheduler.snapshot()["counters"]["recoveries"] == 1
    finally:
        release.set()
        scheduler.close()


def test_both_pools_bounded_and_cancelled_queue_keeps_reservation():
    cfg = replace(config(), quality_queue=1)
    scheduler = AdaptiveScheduler(cfg)
    entered, release, cancel = threading.Event(), threading.Event(), threading.Event()

    def blocked(route, reason, wait):
        entered.set()
        assert release.wait(2)
        return ChunkResult(route)

    try:
        first = scheduler.submit(blocked, 350)
        assert entered.wait(1)
        queued = scheduler.submit(lambda *args: pytest.fail("cancelled task computed"), 350,
                                  active=lambda: not cancel.is_set())
        cancel.set()
        fast = scheduler.submit(blocked, 350)
        with pytest.raises(Overloaded):
            scheduler.submit(blocked, 350)
        assert scheduler.snapshot()["pending"] == {"quality": 2, "spacy_sm": 1}
        release.set()
        first.result(timeout=1)
        fast.result(timeout=1)
        with pytest.raises(RequestCancelled):
            queued.result(timeout=1)
        assert scheduler.snapshot()["pending"] == {"quality": 0, "spacy_sm": 0}
    finally:
        release.set()
        scheduler.close()


def test_budget_and_request_deadline_admit_fast_before_quality_queue_is_full():
    clock = [10.0]
    scheduler = AdaptiveScheduler(config(quality_initial_ms=200), clock=lambda: clock[0])
    try:
        result = scheduler.submit(lambda route, reason, wait: ChunkResult(route), 350,
                                  deadline=10.02).result(timeout=1)
        assert result.chunk_id == "spacy_sm"
        assert scheduler.snapshot()["counters"]["reason_quality_budget"] == 1
        with pytest.raises(RequestCancelled):
            scheduler.submit(lambda *a: None, 350, deadline=10.0)
    finally:
        scheduler.close()


class Gate:
    def __init__(self, score=1.0, fail=False):
        self.value, self.fail, self.inputs = score, fail, []

    def score(self, text):
        self.inputs.append(text)
        if self.fail:
            raise ValueError("must not expose payload")
        return self.value


class Ner:
    def __init__(self, fail=False):
        self.inputs, self.fail = [], fail

    def run(self, text):
        self.inputs.append(text)
        if self.fail:
            raise ValueError("must not expose payload")
        at = text.find("Иван")
        return [] if at < 0 else [dict(type="FULL_NAME", start=at, end=at+4, text="Иван")]


def test_fast_uses_original_even_when_residual_is_blank_and_never_calls_gate():
    gate, large, fast = Gate(), Ner(), Ner()
    detector = PIIDetector(config(backend="spacy_sm"), gate=gate, ner=large, spacy=fast, warmup=False)
    try:
        original = "👩‍💻 Иван"
        result = detector.detect_chunk_v2("1", original, " " * len(original.encode()))
        assert fast.inputs == [original]
        assert not gate.inputs and not large.inputs
        assert not result.audit.gate_ran
        assert original[result.entities[0].start:result.entities[0].end] == "Иван"
    finally:
        detector.close()


@pytest.mark.parametrize("score,fail,expects_ner", [(0.0, False, False), (1.0, False, True), (0.0, True, True), (float('nan'), False, True)])
def test_quality_preserves_gate_semantics_and_error_fallback(score, fail, expects_ner):
    gate, ner = Gate(score, fail), Ner()
    detector = PIIDetector(config(backend="quality"), gate=gate, ner=ner, warmup=False)
    try:
        result = detector.detect_chunk_v2("1", "Иван", "****")
        assert gate.inputs == ["****"]
        assert ner.inputs == (["Иван"] if expects_ner else [])
        assert result.error_code == "NONE"
        if fail:
            assert result.audit.gate_error == "ValueError"
    finally:
        detector.close()


def test_inference_failure_is_not_clean_and_does_not_race_other_backend():
    fast = Ner()
    detector = PIIDetector(config(backend="quality"), gate=Gate(), ner=Ner(True), spacy=fast, warmup=False)
    try:
        result = detector.detect_chunk_v2("1", "Иван", "Иван")
        assert result.error_code == "MODEL_ERROR"
        assert result.audit.error == "ValueError"
        assert not fast.inputs
    finally:
        detector.close()


def test_partial_batch_rejection_is_an_error_and_releases_all_admissions():
    release = threading.Event()
    class BlockedNer(Ner):
        def run(self, text):
            assert release.wait(2)
            return super().run(text)
    detector = PIIDetector(config(backend="spacy_sm"), spacy=BlockedNer(), warmup=False)
    try:
        with pytest.raises(Overloaded):
            detector.detect_batch_v2("b", "unicode_code_points", [("1", "Иван", "Иван"), ("2", "Иван", "Иван")])
    finally:
        release.set()
        detector.close()
    assert detector.scheduler.snapshot()["pending"]["spacy_sm"] == 0


@pytest.mark.parametrize("changes", [dict(backend="bogus"), dict(quality_workers=0), dict(recovery_ratio=1), dict(routing_budget_ms=float('nan')), dict(spacy_queue=-1), dict(grpc_max_workers=1)])
def test_invalid_settings_fail_before_loading_models(changes):
    with pytest.raises(ValueError):
        replace(Config(), **changes)


def test_stale_slow_ewma_can_recover_via_idle_quality_probe():
    clock = [10.0]
    scheduler = AdaptiveScheduler(config(quality_initial_ms=200), clock=lambda: clock[0])
    try:
        first = scheduler.submit(lambda route, reason, wait: ChunkResult(route), 350)
        assert first.result(timeout=1).chunk_id == 'spacy_sm'
        for _ in range(8):
            clock[0] += 1
            probe = scheduler.submit(lambda route, reason, wait: ChunkResult(route), 350)
            assert probe.result(timeout=1).chunk_id == 'quality'
        assert scheduler.snapshot()['counters']['reason_recovery_probe'] >= 1
        assert scheduler.snapshot()['degraded'] is False
    finally:
        scheduler.close()
