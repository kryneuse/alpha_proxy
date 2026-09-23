"""CPU spaCy runtime with exclusive ownership of each loaded Language object."""
from __future__ import annotations

import queue
from pathlib import Path

from . import threading_setup  # noqa: F401
from . import spacy_multibio
from .v14_types import TYPES


class SpacyNerPool:
    def __init__(self, model_dir, workers):
        self._available = queue.Queue(maxsize=workers)
        for _ in range(workers):
            nlp = spacy_multibio.load(Path(model_dir))
            if tuple(nlp.get_pipe("pii_multibio").labels) != tuple(TYPES):
                raise ValueError("spaCy checkpoint labels differ from the service schema")
            nlp("Тестовый клиент: Иван Тестов.")
            self._available.put(nlp)

    def run(self, text):
        nlp = self._available.get()
        try:
            return spacy_multibio.entities(nlp(text))
        finally:
            self._available.put(nlp)
