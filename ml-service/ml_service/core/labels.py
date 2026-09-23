"""Label mappings for the gate (LLAIM) and the main NER (RedMadRobot).

The gate has a fixed set of allowed native types. The main NER maps its native
labels to target entity types; only native labels that actually exist in the
model are mapped (we never invent classes the model does not have).
"""
from __future__ import annotations

from typing import Dict, FrozenSet

# ---------------------------------------------------------------------------
# LLAIM gate: allowed native types -> target entity types.
# ---------------------------------------------------------------------------
# Allowed native types that can open the gate and contribute to gate_score.
GATE_ALLOWED_TYPES: Dict[str, str] = {
    "PER": "FULL_NAME",
    "ADDRESS": "ADDRESS",
    "INN": "INN",
    "PASSPORT": "PASSPORT",
    "PHONE": "PHONE",
    "EMAIL": "EMAIL",
}

# Native types that must be ignored: they never open the gate and never enter
# the gate_score maximum.
GATE_IGNORED_TYPES: FrozenSet[str] = frozenset(
    {
        "ORG",
        "OGRN",
        "SNILS",
        "CASE_NUMBER",
        "BANK_ACCOUNT",
        "DATE",
        "POSITION",
    }
)

# ---------------------------------------------------------------------------
# RedMadRobot main NER: native labels -> target entity types.
# ---------------------------------------------------------------------------
# Only native labels present in the model config are mapped. Types in the
# target list that have no native label (e.g. DATE_OF_BIRTH, CVV, PIN) are
# simply never produced by this model.
RMR_NATIVE_TO_TARGET: Dict[str, str] = {
    "PASSPORT": "PASSPORT",
    "CREDIT_CARD": "CARD_NUMBER",
    "DRIVER_LICENSE": "DRIVER_LICENSE",
    "INN": "INN",
    "CITY": "CITY",
    # COUNTRY has no proto enum value; per product decision it is emitted as
    # ADDRESS (a separate entity, never merged with other address components).
    "COUNTRY": "ADDRESS",
    "STREET": "STREET",
    "HOUSE": "HOUSE",
    "EMAIL": "EMAIL",
    "PHONE": "PHONE",
    # Name components are merged into FULL_NAME during normalization.
    "FIRST_NAME": "FULL_NAME",
    "LAST_NAME": "FULL_NAME",
    "MIDDLE_NAME": "FULL_NAME",
}

# Native labels that exist in the model but are not in the target list.
# They are filtered out by the enabled_types filter.
RMR_NON_TARGET_NATIVE: FrozenSet[str] = frozenset(
    {
        "SNILS",
        "OMS",
        "MILITARY_ID",
        "BIRTH_CERTIFICATE",
        "REGION",
        "DISTRICT",
        "URL",
        "IP_ADDRESS",
    }
)

# Name-component native labels that are merged into a single FULL_NAME entity.
RMR_NAME_COMPONENTS: FrozenSet[str] = frozenset(
    {"FIRST_NAME", "LAST_NAME", "MIDDLE_NAME"}
)