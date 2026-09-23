"""Bounded, load-aware admission to independent quality and fast worker pools.

The pending count includes running AND queued tasks, including cancelled work
until its wrapper is dequeued. Executor cancellation cannot create an unbounded
backlog of cancelled queue entries. No speculative duplicate inference is used.
"""
from __future__ import annotations

import threading
import time
from collections import Counter
from concurrent.futures import ThreadPoolExecutor


class Overloaded(Exception):
    pass


class RequestCancelled(Exception):
    pass


class AdaptiveScheduler:
    def __init__(self, config, clock=time.monotonic):
        self.config = config
        self.clock = clock
        self._lock = threading.Lock()
        self._closed = False
        self._degraded_until = 0.0
        self._degraded = False
        self._pending = dict(quality=0, spacy_sm=0)
        self._running = dict(quality=0, spacy_sm=0)
        self._units = dict(quality=0.0, spacy_sm=0.0)
        self._ewma = dict(quality=config.quality_initial_ms, spacy_sm=config.spacy_initial_ms)
        self._counts = Counter()
        self._queue_ms = Counter()
        self._compute_ms = Counter()
        self._workers = dict(quality=config.quality_workers, spacy_sm=config.spacy_workers)
        self._capacity = dict(
            quality=config.quality_workers + config.quality_queue,
            spacy_sm=config.spacy_workers + config.spacy_queue,
        )
        routes = ("quality", "spacy_sm") if config.backend == "adaptive" else (config.backend,)
        self._pools = {
            route: ThreadPoolExecutor(max_workers=self._workers[route], thread_name_prefix=f"pii-{route}")
            for route in routes
        }

    def _estimate(self, route, units):
        # Conservative estimate including work already running and this chunk.
        return self._ewma[route] * (self._units[route] / self._workers[route] + units)

    def submit(self, function, codepoints, *, active=lambda: True, deadline=None):
        if not active():
            raise RequestCancelled()
        now = self.clock()
        if deadline is not None and now >= deadline:
            raise RequestCancelled()
        units = max(0.25, codepoints / self.config.max_chunk_codepoints)
        with self._lock:
            if self._closed:
                raise Overloaded("scheduler_closed")
            budget = self.config.routing_budget_ms
            if deadline is not None:
                budget = min(budget, max(0.0, (deadline - now) * 1000))
            quality_room = self._pending["quality"] < self._capacity["quality"]
            quality_budget = self._estimate("quality", units) <= budget
            if self.config.backend == "adaptive":
                if (self._degraded and now >= self._degraded_until
                        and self._pending["quality"] <= self.config.quality_recovery_pending
                        and self._estimate("quality", units) <= budget * self.config.recovery_ratio):
                    self._degraded = False
                    self._counts["recoveries"] += 1
                if not self._degraded and (not quality_room or not quality_budget):
                    self._degraded = True
                    self._degraded_until = now + self.config.recovery_hold_ms / 1000
                    self._counts["degradations"] += 1
                # A stale slow EWMA must not trap the service in fast mode
                # forever after load drops. Admit one idle quality probe per
                # hold period, still respecting the actual request deadline.
                probe = (self._degraded and now >= self._degraded_until
                         and self._pending["quality"] == 0
                         and (deadline is None or self._estimate("quality", units) < (deadline-now)*1000))
                if not self._degraded:
                    route, reason = "quality", "normal"
                elif probe:
                    route, reason = "quality", "recovery_probe"
                    self._degraded_until = now + self.config.recovery_hold_ms / 1000
                else:
                    route = "spacy_sm"
                    reason = "quality_capacity" if not quality_room else "quality_budget" if not quality_budget else "hold"
                    # Do not leave usable quality capacity idle while fast is full.
                    if self._pending[route] >= self._capacity[route] and quality_room and quality_budget:
                        route, reason = "quality", "fast_full_quality_spare"
            else:
                route, reason = self.config.backend, "fixed"
            if self._pending[route] >= self._capacity[route]:
                self._counts["rejected"] += 1
                raise Overloaded("inference_capacity_exhausted")
            self._pending[route] += 1
            self._units[route] += units
            self._counts[f"routed_{route}"] += 1
            self._counts[f"reason_{reason}"] += 1

        def run():
            started = self.clock()
            ran = False
            try:
                if not active() or (deadline is not None and started >= deadline):
                    with self._lock:
                        self._counts["cancelled_before_compute"] += 1
                    raise RequestCancelled()
                with self._lock:
                    self._running[route] += 1
                ran = True
                result = function(route, reason, (started - now) * 1000)
                with self._lock:
                    self._counts[f"completed_{route}"] += 1
                    if result.error_code != "NONE":
                        self._counts[f"errors_{route}"] += 1
                return result
            finally:
                elapsed = max(0.0, (self.clock() - started) * 1000)
                with self._lock:
                    self._pending[route] -= 1
                    self._units[route] = max(0.0, self._units[route] - units)
                    if ran:
                        self._running[route] -= 1
                        floor = self.config.quality_min_ms if route == "quality" else 0.1
                        sample = max(floor, elapsed / units)
                        self._ewma[route] = 0.8 * self._ewma[route] + 0.2 * sample
                        self._queue_ms[route] += (started - now) * 1000
                        self._compute_ms[route] += elapsed

        try:
            return self._pools[route].submit(run)
        except RuntimeError:
            with self._lock:
                self._pending[route] -= 1
                self._units[route] = max(0.0, self._units[route] - units)
            raise Overloaded("scheduler_closed") from None

    def snapshot(self):
        with self._lock:
            return {
                "backend": self.config.backend,
                "degraded": self._degraded,
                "pending": dict(self._pending),
                "running": dict(self._running),
                "capacity": dict(self._capacity),
                "estimated_ms_per_full_chunk": dict(self._ewma),
                "counters": dict(self._counts),
                "queue_ms_sum": dict(self._queue_ms),
                "compute_ms_sum": dict(self._compute_ms),
            }

    def close(self):
        with self._lock:
            self._closed = True
        for pool in self._pools.values():
            pool.shutdown(wait=True, cancel_futures=False)
