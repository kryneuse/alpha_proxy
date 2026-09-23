"""Type schema for the v14a NER head (25 independent BIO sequences).

The order matches model.json.types of source99-expansion-v14a-epoch1-onnx.
The last axis of ner_logits is O=0, B=1, I=2, applied independently per type.
"""
from __future__ import annotations

from typing import Dict, List

TYPES: List[str] = [
    "FULL_NAME",
    "DATE_OF_BIRTH",
    "BIRTH_PLACE",
    "PASSPORT",
    "CITIZENSHIP",
    "PASSPORT_ISSUER",
    "DEPARTMENT_CODE",
    "PASSPORT_ISSUE_DATE",
    "DRIVER_LICENSE",
    "ADDRESS",
    "COUNTRY",
    "POSTCODE",
    "CITY",
    "STREET",
    "HOUSE",
    "APARTMENT",
    "EMAIL",
    "PHONE",
    "INN",
    "CARD_NUMBER",
    "CVV",
    "PIN",
    "CARDHOLDER_NAME",
    "REGION",
    "DISTRICT",
]

TYPE_ID: Dict[str, int] = {t: i for i, t in enumerate(TYPES)}

# Aliases used when mapping rule entities / legacy names into the v14 schema.
ALIASES: Dict[str, str] = {
    "NAME": "FULL_NAME",
    "PERSON": "FULL_NAME",
    "FIRST_NAME": "FULL_NAME",
    "LAST_NAME": "FULL_NAME",
    "MIDDLE_NAME": "FULL_NAME",
    "PHONE_NUMBER": "PHONE",
    "CREDIT_CARD": "CARD_NUMBER",
    "BANK_CARD_NUMBER": "CARD_NUMBER",
    "PASSPORT_NUMBER": "PASSPORT",
    "CVC": "CVV",
    "BIRTH_DATE": "DATE_OF_BIRTH",
    "DATE": "DATE_OF_BIRTH",
    "POSTAL_CODE": "POSTCODE",
    "CARD": "CARD_NUMBER",
    "PLACE_OF_BIRTH": "BIRTH_PLACE",
}