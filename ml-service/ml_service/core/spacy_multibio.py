"""spaCy CNN encoders with independent BIO outputs for overlapping PII types.

Import this module before spacy.load(). The full prediction is doc.spans['pii'];
doc.ents is a lossy, non-overlapping convenience view.
"""
import os

for _key in ('OMP_NUM_THREADS', 'MKL_NUM_THREADS', 'OPENBLAS_NUM_THREADS',
             'VECLIB_MAXIMUM_THREADS', 'NUMEXPR_NUM_THREADS'):
    os.environ.setdefault(_key, '1')

import argparse
import json
import sys
from pathlib import Path

import numpy as np
import spacy
from spacy.language import Language
from spacy.util import registry, filter_spans
from thinc.api import chain, with_array, Linear

from .v14_types import TYPES


@registry.architectures('pii.MultiBIO.v1')
def build_multibio(tok2vec, n_types: int):
    head = with_array(Linear(nO=n_types * 3, nI=tok2vec.get_dim('nO')))
    model = chain(tok2vec, head)
    model.set_ref('encoder', tok2vec)
    model.set_ref('head', head)
    return model


def decode(doc, logits, labels=TYPES):
    tags = logits.reshape(len(doc), len(labels), 3).argmax(axis=-1)
    spans = []
    for j, label in enumerate(labels):
        start = None
        for i in range(len(doc) + 1):
            tag = int(tags[i, j]) if i < len(doc) else 0
            if start is not None and tag != 2:
                # Whitespace-only edges are never part of the entity value.
                a, b = start, i
                while a < b and doc[a].is_space:
                    a += 1
                while b > a and doc[b - 1].is_space:
                    b -= 1
                if a < b:
                    spans.append(spacy.tokens.Span(doc, a, b, label=label))
                start = None
            if tag == 1 or (tag == 2 and start is None):
                start = i
    return sorted(spans, key=lambda s: (s.start_char, s.end_char, s.label_))


class PiiMultiBIO:
    def __init__(self, nlp, name, model, labels):
        self.vocab = nlp.vocab
        self.name = name
        self.model = model
        self.labels = tuple(labels)

    def __call__(self, doc):
        if len(doc):
            scores = self.model.predict([doc])[0]
            spans = decode(doc, scores, self.labels)
        else:
            spans = []
        doc.spans['pii'] = spans
        doc.ents = filter_spans(spans)
        return doc

    def pipe(self, docs, batch_size=32, **kwargs):
        from spacy.util import minibatch
        for batch in minibatch(docs, size=batch_size):
            if not any(len(d) for d in batch):
                for doc in batch:
                    yield self(doc)
                continue
            scores = self.model.predict(batch)
            for doc, score in zip(batch, scores, strict=True):
                spans = decode(doc, score, self.labels) if len(doc) else []
                doc.spans['pii'] = spans
                doc.ents = filter_spans(spans)
                yield doc

    def to_disk(self, path, *, exclude=()):
        path = Path(path)
        path.mkdir(parents=True, exist_ok=True)
        (path / 'model.bin').write_bytes(self.model.to_bytes())
        (path / 'labels.json').write_text(json.dumps(self.labels))
        return self

    def from_disk(self, path, *, exclude=()):
        path = Path(path)
        labels = tuple(json.loads((path / 'labels.json').read_text()))
        if labels != self.labels:
            raise ValueError('Configured labels differ from the saved model')
        self.model.from_bytes((path / 'model.bin').read_bytes())
        return self


@Language.factory('pii_multibio', default_config={'labels': TYPES})
def create_multibio(nlp, name, model, labels):
    return PiiMultiBIO(nlp, name, model, labels)


def entities(doc):
    return [dict(type=s.label_, start=s.start_char, end=s.end_char, text=s.text)
            for s in doc.spans.get('pii', ())]


def load(model):
    spacy.require_cpu()
    return spacy.load(model)


class SpacyNER:
    """Adapter with the same run(text) entity interface as the ONNX NER."""
    def __init__(self, model):
        self.nlp = load(model)

    def run(self, text):
        return entities(self.nlp(text))

