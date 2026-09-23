> Новый режим: **adaptive = gate1-v3 + v14a при свободном quality-пуле, spaCy sm при нагрузке**. Несколько CPU-реплик, запуск на Mac, маскирование/демаскирование и метрики описаны в [ADAPTIVE.md](../docs/ADAPTIVE.md). Исторические инструкции ниже могут описывать только прежний quality-путь.

# ML PII Detection Service

Python-микросервис для обнаружения персональных данных (PII) в тексте.
Реализует gRPC-контракт `PIIDetector` (см. `ml_service/proto/pii.proto`).

## Архитектура

```
(original_text, gate_text) → gate1-v3 → NER v14a → repair structured-v3 → фильтр целевых типов
```

1. **Гейт** (`gate1-v3-onnx`, ONNX dynamic INT8) — быстрый фильтр на
   `gate_text` (byte-preserving masked residual). Решает, есть ли в остатке
   персональные данные. При закрытом гейте основной NER не вызывается.
2. **Основной NER** (`source99-expansion-v14a-epoch1-onnx`, ONNX dynamic INT8) —
   извлекает сущности из `original_text` соответствующего чанка (включая то,
   что уже нашли правила).
3. **Repair structured-v3** (`core/v14_repair.py`) — консервативная правка
   границ: расширяет существующие span'ы, никогда не выдумывает тип.
   Сборка адреса выключена.
4. **Фильтр целевых типов** — применяется к итоговым сущностям.

Гейт и NER — отдельные артефакты. Внутренний `gate_threshold` в `model.json`
NER-графа **не** является порогом каскада; используется явный порог ниже.

## Модели

| Роль | Артефакт | ONNX | Квантизация | SHA256 (model.int8.onnx) |
|------|----------|------|-------------|--------------------------|
| Гейт | `gate1-v3-onnx` | `model.int8.onnx` | dynamic INT8 | `e02c5067c73b9f773b973d05797a00b56f1fbad21c60bdb170e7f2d326dcce65` |
| NER | `source99-expansion-v14a-epoch1-onnx` | `model.int8.onnx` | dynamic INT8 | `0884223eaec3ceb4be4b196df0aafd641e23a728996c885f45820ff799262185` |

- NER: 98 625 104 параметра, 25 выходных типов, head `[batch, seq, 25, 3]`
  (последняя ось O/B/I, независимо на тип).
- Гейт: `gate_logits` float `[batch]`, score = float32 sigmoid, максимум по
  перекрывающимся окнам.
- Ревизии и контрольные суммы артефактов зафиксированы в
  `models/manifest.json`.

### Порог каскада

Порог гейта: `1.823714370630114e-7` (сравнение `score >= threshold`).
Значение зафиксировано в `ml_service/config.py` (`DEFAULT_GATE_THRESHOLD`) и
переопределяется через `ML_GATE_THRESHOLD`.

## Установка

```bash
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

## Запуск

```bash
.venv/bin/python -m ml_service
```

Сервер слушает gRPC на порту `50051` (настраивается через `ML_GRPC_PORT`).

### Конфигурация (переменные окружения)

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `ML_MODELS_DIR` | `models/` | Директория с ONNX-моделями |
| `ML_NER_DIR` | `models/source99-expansion-v14a-epoch1-onnx` | Директория NER-артефакта |
| `ML_GATE_DIR` | `models/gate1-v3-onnx` | Директория гейт-артефакта |
| `ML_GATE_THRESHOLD` | `1.823714370630114e-7` | Порог гейта |
| `ML_NUM_WORKERS` | `4` | Число обработчиков gRPC |
| `ML_INTRA_OP_THREADS` | `1` | ORT intra-op потоки |
| `ML_INTER_OP_THREADS` | `1` | ORT inter-op потоки |
| `ML_TENSOR_BATCH_SIZE` | `1` | Размер тензорного батча |
| `ML_GRPC_PORT` | `50051` | Порт gRPC |
| `ML_MODEL_VERSION` | `v14a-epoch1-gate1-v3` | Версия модели в ответе |

## Parity-проверка против reference LeanModel

Скрипт `scripts/parity_check.py` сравнивает `LeanGate`/`LeanNer` из
`ml_service/core/onnx_model.py` с reference `LeanModel` из pii-lab на одних и
тех же парах (реальные чанки из `reports/rules_e2e_v1/chunks.json`):
gate score (float32 sigmoid, max по окнам) и NER spans (type, start, end) до
repair.

```bash
# Требуется venv с razdel (reference e2e_runtime импортирует razdel).
/Users/aekuleshevskii/alpha_test_pd/pii-lab/.venv-spacy/bin/python \
  scripts/parity_check.py [--limit N]
```

По умолчанию проверяются все 2210 пар; ожидается точное совпадение
(`gate_max_score_difference == 0`, `ner_span_disagreements == []`).

## Параллелизм

- Бюджет: 4 обработчика × 1 вычислительный поток, `tensor_batch_size=1`.
- ORT: `intra_op=1`, `inter_op=1`, spinning выключен.
- Потоки вычислительных библиотек ограничены до инициализации
  (`ml_service/core/threading_setup.py`).
- Модели загружаются один раз и разделяются между потоками (ORT-сессии
  потокобезопасны).
- Токенизаторы не потокобезопасны (Rust `tokenizers`), поэтому каждый вызов
  создаёт собственный encoding; объекты `Tokenizer` используются read-only.

## Тесты

```bash
.venv/bin/python -m pytest
```

## Структура

```
ml-service/
├── ml_service/
│   ├── config.py              # конфигурация (порог, пути, версия)
│   ├── core/
│   │   ├── detector.py        # гейт + NER + фильтр
│   │   ├── onnx_model.py      # LeanGate / LeanNer (ONNX Runtime)
│   │   ├── v14_repair.py      # repair structured-v3
│   │   ├── v14_types.py       # 25 типов NER v14a
│   │   ├── types.py           # типы результата
│   │   └── threading_setup.py # ограничение потоков
│   ├── proto/                 # сгенерированный gRPC-код
│   └── server/                # gRPC-сервер
├── models/                    # ONNX-модели и манифест
├── scripts/                   # parity-проверка, клиент
└── tests/                     # тесты
```