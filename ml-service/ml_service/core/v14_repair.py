"""Conservative boundary repair for the v14a NER (structured-v3).

This is a direct port of the reference postprocess.repair_structured_v3 and the
functions it calls. It expands existing model spans, never invents a type.
Address assembly is off.
"""
from __future__ import annotations

import re
from typing import Dict, List

NUMERIC = {
    "PASSPORT", "DRIVER_LICENSE", "INN", "CARD_NUMBER", "CVV", "PIN",
    "PHONE", "DEPARTMENT_CODE", "POSTCODE", "HOUSE", "APARTMENT",
}
DATES = {"DATE_OF_BIRTH", "PASSPORT_ISSUE_DATE"}
WORDS = {
    "FULL_NAME", "CARDHOLDER_NAME", "CITY", "COUNTRY", "CITIZENSHIP",
    "BIRTH_PLACE", "PASSPORT_ISSUER", "REGION", "DISTRICT", "STREET",
}
NUMBER_LENGTHS = {
    "PASSPORT": {10},
    "DRIVER_LICENSE": {10},
    "INN": {10, 12},
    "CARD_NUMBER": set(range(13, 20)),
    "PHONE": set(range(7, 16)),
    "DEPARTMENT_CODE": {6},
    "POSTCODE": {6},
    "CVV": {3, 4},
    "PIN": {4, 5, 6},
}
DIGIT_WORD = r"(?:ноль|нуль|один|два|три|четыре|пять|шесть|семь|восемь|девять)"


def repair_structured_v3(text: str, entities: List[Dict]) -> List[Dict]:
    """V3 preserves letter-bearing document IDs instead of trimming their prefix.

    Only an already predicted passport or licence may be expanded to its
    enclosing compact alphanumeric identifier.
    """
    words = [
        m
        for m in re.finditer(r"(?<!\w)[A-Za-zА-Яа-яЁё0-9]{6,20}(?!\w)", text)
        if 1 <= sum(c.isalpha() for c in m.group()) <= 4
        and sum(c.isdigit() for c in m.group()) >= 5
    ]
    protected: List[Dict] = []
    regular: List[Dict] = []
    for original in entities:
        e = original.copy()
        a, b = e["start"], e["end"]
        if e["type"] not in ["PASSPORT", "DRIVER_LICENSE"]:
            regular.append(e)
            continue
        overlaps = [m for m in words if m.start() < b and m.end() > a]
        if len(overlaps) == 1:
            a = min(a, overlaps[0].start())
            b = max(b, overlaps[0].end())
            e.update(start=a, end=b, text=text[a:b])
            protected.append(e)
        elif any(c.isalpha() for c in text[a:b]):
            protected.append(e)
        else:
            regular.append(e)
    result = repair_structured(text, regular) + protected
    return list({(e["type"], e["start"], e["end"]): e for e in result}.values())


def repair_structured(text: str, entities: List[Dict]) -> List[Dict]:
    """Snap existing predictions to complete value runs.

    Never creates a type without a model prediction. Spoken single-digit
    sequences are supported only for CVV/PIN; surrounding context is still the
    model's job.
    """
    numbers = list(re.finditer(r"(?<!\w)[+\d](?:[\d\s()./№–—-]*\d)?", text))
    spoken = list(
        re.finditer(
            r"\b" + DIGIT_WORD + r"(?:[\s,;-]+" + DIGIT_WORD + r")*\b",
            text,
            re.IGNORECASE,
        )
    )
    result: List[Dict] = []
    for original in entities:
        e = original.copy()
        a, b = e["start"], e["end"]
        typ = e["type"]
        spoken_match = False
        if typ in ["CVV", "PIN"]:
            candidates = [
                m
                for m in spoken
                if m.start() < b
                and m.end() > a
                and len(re.findall(DIGIT_WORD, m.group(), re.IGNORECASE)) in NUMBER_LENGTHS[typ]
            ]
            if len(candidates) == 1:
                a, b = candidates[0].span()
                e.update(start=a, end=b, text=text[a:b])
                result.append(e)
                spoken_match = True
        if spoken_match:
            continue
        if typ in NUMBER_LENGTHS:
            candidates = [
                m
                for m in numbers
                if m.start() < b
                and m.end() > a
                and sum(c.isdigit() for c in m.group()) in NUMBER_LENGTHS[typ]
            ]
            if len(candidates) == 1:
                a, b = candidates[0].span()
                e.update(start=a, end=b, text=text[a:b])
        if typ in WORDS:
            while b > a and text[b - 1] in '"\'«»':
                b -= 1
            e.update(start=a, end=b, text=text[a:b])
        result.extend(repair(text, [e]))
    unique = {(e["type"], e["start"], e["end"]): e for e in result}
    return list(unique.values())


def repair(text: str, entities: List[Dict]) -> List[Dict]:
    result: List[Dict] = []
    for original in entities:
        e = original.copy()
        a, b = e["start"], e["end"]
        typ = e["type"]
        if typ in NUMERIC:
            while a < b and not (text[a].isdigit() or text[a] in "+•*"):
                a += 1
            while b > a and text[b - 1] in " ,;:!.?":
                b -= 1
            while a > 0 and text[a - 1].isdigit() and a < b and text[a].isdigit():
                a -= 1
            while b < len(text) and text[b].isdigit() and a < b and text[b - 1].isdigit():
                b += 1
        if typ in DATES:
            candidates = list(
                re.finditer(r"(?<!\d)\d{1,4}\s*[./-]\s*\d{1,2}\s*[./-]\s*\d{2,4}(?!\d)", text)
            )
            overlaps = [m for m in candidates if m.start() < b and m.end() > a]
            if len(overlaps) == 1:
                a, b = overlaps[0].span()
        if typ in WORDS:
            while a > 0 and text[a - 1].isalpha() and a < b and text[a].isalpha():
                a -= 1
            while b < len(text) and text[b].isalpha() and a < b and text[b - 1].isalpha():
                b += 1
            while b > a and text[b - 1] in " ,;:!?":
                b -= 1
        if a >= b:
            continue
        if typ == "CVV" and not 3 <= sum(c.isdigit() for c in text[a:b]) <= 4:
            continue
        if typ == "PIN" and not 4 <= sum(c.isdigit() for c in text[a:b]) <= 6:
            continue
        e.update(start=a, end=b, text=text[a:b])
        result.append(e)
    final: List[Dict] = []
    for typ in sorted({e["type"] for e in result}):
        seq = sorted((e for e in result if e["type"] == typ), key=lambda e: (e["start"], e["end"]))
        for e in seq:
            if final and final[-1]["type"] == typ and e["start"] < final[-1]["end"]:
                final[-1]["end"] = max(final[-1]["end"], e["end"])
                final[-1]["text"] = text[final[-1]["start"] : final[-1]["end"]]
            else:
                final.append(e)
    return final