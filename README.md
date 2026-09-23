> Новый режим: **adaptive = gate1-v3 + v14a при свободном quality-пуле, spaCy sm при нагрузке**. Несколько CPU-реплик, запуск на Mac, маскирование/демаскирование и метрики описаны в [ADAPTIVE.md](docs/ADAPTIVE.md). Исторические инструкции ниже могут описывать только прежний quality-путь.

# ALFAGEN Rule Engine

Deterministic rule engine for detecting personal data (ПД) in Russian-language
text. Native Go implementation, no ML, no external calls at runtime.

## Architecture

```
text -> normalization -> deterministic recognizers -> context scoring -> span resolver -> []Entity
```

Pipeline stages:

1. **Normalization** (`internal/normalize`) — lowercases, replaces unicode
   dashes with ASCII `-`, collapses whitespace. Crucially it keeps a per-rune
   mapping back to the **original** byte offsets, so spans are always reported
   relative to the input text. Normalization never reorders or drops runes.

2. **Recognizers** (`internal/recognizer`) — each entity type has its own
   recognizer implementing the `Recognizer` interface. Recognizers use regex,
   checksums, dictionaries and context constructions. They return
   `CandidateSpan` values with type, text, original offsets, score, sources
   and reason.

3. **Context scoring** (`internal/context`) — inspects the text window around
   each candidate and boosts/suppresses its score based on keywords. It also
   disambiguates types (e.g. a date near "выдан" becomes `PASSPORT_ISSUE_DATE`,
   a 4+6 digit number near driver keywords becomes `DRIVER_LICENSE`).

4. **Span resolver** (`internal/resolver`) — drops candidates below a score
   threshold, removes duplicates, resolves overlaps preferring the more
   specific/validated candidate, and emits typed `Entity` values.

The engine (`internal/engine`) wires the pipeline together. Masking/policy is
a separate layer and is **not** part of this package.

## Supported entity types (17)

`FULL_NAME`, `BIRTH_DATE`, `BIRTH_PLACE`, `PASSPORT`, `CITIZENSHIP`,
`PASSPORT_ISSUER`, `DEPARTMENT_CODE`, `PASSPORT_ISSUE_DATE`, `DRIVER_LICENSE`,
`ADDRESS`, `EMAIL`, `PHONE`, `INN`, `CARD_NUMBER`, `CVV`, `PIN`,
`CARDHOLDER_NAME`.

## Recognizers

| Recognizer | Method | Notes |
|---|---|---|
| Email | regex | standard email pattern |
| Phone | regex | `+7/8/7` with separators, parens, dashes |
| INN | regex + checksum | real 10- and 12-digit control-digit validation |
| Card number | regex + Luhn | 16 digits, Luhn validated; bare digit runs rejected |
| CVV | regex + context | 3 digits only in banking context |
| PIN | regex + context | 4 digits only in banking context |
| Cardholder | regex + context | latin name in banking context |
| Passport | regex + format | `XXXX XXXXXX` and `серия XX XX номер XXXXXX` |
| Department code | regex + context | `XXX-XXX` only in passport context |
| Driver license | regex + context | 4+6 digits near driver keywords |
| Passport issuer | regex + context | "выдан ..." constructions |
| Date | regex + format | numeric and word forms; context decides birth vs issue |
| Citizenship | dictionary | country/citizenship dictionary |
| Birth place | regex + context | "место рождения", "родился в ..." |
| Full name | regex + dictionary + heuristics | context, name dict, patronymic/last-name endings |
| Address | regex + context + structure | requires address structure, not just a city |

## Validators / checksums

- **INN**: real control-digit algorithm for 10- and 12-digit INN.
- **Card number**: Luhn algorithm.
- **Passport / driver license / department code**: format + context validation
  (no reliable checksum exists for these documents, so none is invented).

## Hard negatives handled

- "Александр Сергеевич Пушкин" in a literary context is not a client name.
- A bank branch address is not a personal address.
- A 16-digit order number is not a card number.
- "Код доступа 7305" is not a PIN.
- "Аудитория 314" is not a CVV.
- An ordinary event date is not a birth date.

## Usage

```go
import "github.com/alpha-proxy/rule-engine/internal/engine"

e := engine.New(engine.Options{MinScore: 0.5})
entities := e.Analyze("Клиент: Иванов Иван Петрович, ИНН 7707083893")
```

## Running tests

```sh
go test ./...
```

## Running evaluation

```sh
go run ./cmd/evaluate
```

Prints overall and per-type Precision / Recall / F1, false positive rate on
negative cases, and average latency. The dataset lives in
`internal/eval/dataset.go` (all synthetic).

## Running the ML cascade (Go + Python)

The Go server delegates residual PII detection to the Python ML service over
gRPC. The ML service runs the `gate1-v3` gate and the `source99-expansion-v14a-epoch1`
NER (see `ml-service/README.md`).

### 1. Start the Python ML service

```bash
cd ml-service
.venv/bin/python -m ml_service
```

Exact models and gate threshold (defaults in `ml_service/config.py`):

| Параметр | Значение |
|----------|----------|
| Гейт | `models/gate1-v3-onnx` (`model.int8.onnx`) |
| NER | `models/source99-expansion-v14a-epoch1-onnx` (`model.int8.onnx`) |
| Порог гейта | `1.823714370630114e-7` (`ML_GATE_THRESHOLD`) |
| gRPC порт | `50051` (`ML_GRPC_PORT`) |

### 2. Start the Go server

```sh
ALPHA_PROXY_ML_ADDR=127.0.0.1:50051 go run ./cmd/server
```

`ALPHA_PROXY_ML_ADDR` defaults to `127.0.0.1:50051`.

## Running benchmarks

```sh
go test ./internal/engine/ -bench=. -benchmem -run=^$
```

## Extension points

The architecture is designed so the following can be added without rewriting
the core:

- **ML recognizer** — implement the `Recognizer` interface and register it.
- **External surname dictionary** — pass a larger `*dict.Dict` to
  `NewFullNameRecognizer`.
- **Full country/citizenship reference** — pass a larger `*dict.Dict` to
  `NewCitizenshipRecognizer`.
- **Additional validators** — add checksum functions and wire them into a
  recognizer.
- **Additional context rules** — extend `context.Scorer`.

## Limitations / known weaknesses

- Full-name detection relies on a small built-in name dictionary plus Russian
  ending heuristics; recall improves with a larger external dataset.
- Address detection requires explicit address keywords and structure; free-form
  addresses without keywords are not detected.
- Passport issuer capture is letter-only and stops at field separators; complex
  issuer strings with embedded digits may be truncated.
- CVV/PIN require banking context; a bare 3/4-digit number is never emitted.
- The country dictionary is a seed; a full reference is a future addition.
- Date disambiguation uses a fixed context window; very long sentences with
  distant keywords may be misclassified.