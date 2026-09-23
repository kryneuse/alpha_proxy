"""Reference inference for LLAIMlegal/ru-legal-ner (v2.0).

Greedy BIO decoding + boundary post-processing. Dependencies: torch, transformers.

    from llaim_ner import load, extract
    tokenizer, model = load()                      # revision="v1.0" for the original release
    for e in extract("ООО «Ромашка», ИНН 7701234567", tokenizer, model):
        print(e["label"], e["text"], e["start"], e["end"])

Why post-processing: a token classifier sometimes stops an entity in the middle of a
word (an account number cut after 3 digits, an e-mail cut after 2 letters). Entities almost
never end mid-word in the training data, so a found entity is extended to the end of the
whitespace-delimited word; then a trailing sentence period/comma is trimmed. The step only
adjusts boundaries and never creates new entities (overlap-match metrics are unchanged;
strict exact-match F1 rises 0.749 -> 0.821 on the held-out set).
"""
import re
from typing import Dict, List

import torch
from transformers import AutoModelForTokenClassification, AutoTokenizer

MODEL_ID = "LLAIMlegal/ru-legal-ner"
ID_TYPES = {"INN", "OGRN", "BANK_ACCOUNT", "EMAIL", "PHONE", "SNILS"}
ID_SEPARATORS = ",;|/"
LINE_BREAKS = "\n\r  "


def load(model_id: str = MODEL_ID, revision: str = None):
    tokenizer = AutoTokenizer.from_pretrained(model_id, revision=revision)
    model = AutoModelForTokenClassification.from_pretrained(model_id, revision=revision).eval()
    return tokenizer, model


def _chunks(text: str, max_chars: int = 900):
    """Split long text at sentence/line boundaries so the model never sees more than ~384 tokens."""
    if len(text) <= max_chars:
        yield 0, text
        return
    start, n = 0, len(text)
    while start < n:
        end = min(start + max_chars, n)
        if end < n:
            b = max(text.rfind(". ", start, end), text.rfind("\n", start, end), text.rfind("; ", start, end))
            if b > start:
                end = b + 1
        yield start, text[start:end]
        start = end


@torch.no_grad()
def _greedy_spans(text: str, tokenizer, model, max_length: int = 384) -> List[Dict]:
    id2label = model.config.id2label
    spans = []
    for offset, chunk in _chunks(text):
        enc = tokenizer(chunk, return_offsets_mapping=True, truncation=True, max_length=max_length,
                        return_tensors="pt")
        offsets = enc.pop("offset_mapping")[0].tolist()
        labels = [id2label[i] for i in model(**enc).logits.argmax(-1)[0].tolist()]
        cur = None
        for (s, e), lab in zip(offsets, labels):
            if s == e:
                continue
            if lab.startswith("B-"):
                if cur:
                    spans.append(cur)
                cur = {"start": s + offset, "end": e + offset, "label": lab[2:]}
            elif lab.startswith("I-") and cur and lab[2:] == cur["label"]:
                cur["end"] = e + offset
            else:
                if cur:
                    spans.append(cur)
                cur = None
        if cur:
            spans.append(cur)
    return spans


def _keep_period(text: str, s: int, e: int, label: str) -> bool:
    if label in ID_TYPES or label in ("CASE_NUMBER", "PASSPORT"):
        return False
    m = re.search(r"([^\W\d_]+)$", text[s:e - 1])
    word = m.group(1) if m else ""
    if not word:
        return False
    if label == "PER":
        return len(word) == 1 and word.isupper()          # initials: "А.А."
    if label == "DATE":
        return word.lower() in ("г", "гг")                # "2026 г."
    return len(word) <= 3                                 # "г.", "ул."


def _trim(text: str, s: int, e: int, label: str):
    if label != "ADDRESS":
        for k in range(s, e):
            if text[k] in LINE_BREAKS:
                e = k
                break
    while e > s:
        c = text[e - 1]
        if c.isspace() or c in ",;:":
            e -= 1
        elif c == "." and not _keep_period(text, s, e, label):
            e -= 1
        elif c == ")" and "(" not in text[s:e]:
            e -= 1
        else:
            break
    while s < e and text[s].isspace():
        s += 1
    return s, e


def postprocess(text: str, spans: List[Dict]) -> List[Dict]:
    spans = sorted(spans, key=lambda x: x["start"])
    out = []
    for k, sp in enumerate(spans):
        s, e, label = sp["start"], sp["end"], sp["label"]
        if e < len(text) and not text[e - 1].isspace() and not text[e].isspace():
            j = e
            while j < len(text) and not text[j].isspace():
                j += 1
            if label in ID_TYPES:
                for m in range(e, j):
                    if text[m] in ID_SEPARATORS:
                        j = m
                        break
            if k + 1 < len(spans):
                j = min(j, spans[k + 1]["start"])
            e = max(e, j)
        s, e = _trim(text, s, e, label)
        if e - s >= 2:
            out.append({"start": s, "end": e, "label": label, "text": text[s:e]})
    return out


def extract(text: str, tokenizer, model, postprocess_boundaries: bool = True) -> List[Dict]:
    spans = _greedy_spans(text, tokenizer, model)
    if postprocess_boundaries:
        return postprocess(text, spans)
    return [dict(s, text=text[s["start"]:s["end"]]) for s in spans]
