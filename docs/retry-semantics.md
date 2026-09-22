# Retry semantics — to be agreed with developer #2

This document formulates the retry semantics that must be agreed with the
developer responsible for the processing layer (developer #2). It is a proposal
for discussion, not a final decision.

## Proposed semantics

1. **Re-sending the original payload** must return the same mask as the first
   request for the same `payload_id`. In other words, processing the same
   original text for the same ID is idempotent with respect to the produced
   mask.

2. **A request carrying a previously issued mask** must return the original
   text. That is, the system must be able to reverse a mask back to the source
   payload.

## Case: mask equals original text

When the mask is identical to the original text, the two rules above produce the
same result: both "mask the original" and "unmask the mask" yield the same
output. The JSON contract is fixed and must not be changed (for example, by
adding an operation flag) to disambiguate this case.

The internal semantics of further retries in this ambiguous case must be agreed
with developer #2 before the processing layer is finalized.