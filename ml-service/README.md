# ML PII Detection Service

Python-микросервис для обнаружения персональных данных (PII) в тексте.
Реализует gRPC-контракт `PIIDetector` (см. `ml_service/proto/pii_detector.proto`).

## Архитектура

```
текст → LLAIM-гейт → RedMadRobot NER → нормализация сущностей → фильтр целевых типов
```

1. **LLAIM-гейт** (`LLAIMlegal/ru-legal-ner`, ONNX FP32) — быстрый фильтр.
   Решает, есть ли в чанке персональные данные. При закрытом гейте основной NER
   не вызывается.
2. **Основной NER** (`redmadrobot-rnd/rubert-base-pii-ner`, ONNX dynamic INT8) —
   извлекает сущности.
3. **Нормализация** — объединение компонентов имени (FIRST/LAST/MIDDLE_NAME)
   в `FULL_NAME`, слияние перекрывающихся фрагментов одного типа.
4. **Фильтр целевых типов** — применяется к итоговым сущностям.

### Соответствие нативных меток целевым типам

Модель RedMadRobot выдаёт нативные метки, которые маппятся в целевые типы.
Типы из целевого списка, отсутствующие у модели (DATE_OF_BIRTH, CVV, PIN и др.),
не производятся. `COUNTRY` не имеет значения в proto-контракте, поэтому по
продуктовому решению отправляется как `ADDRESS` (отдельной сущностью, без
объединения с другими компонентами адреса).

## Модели

| Модель | Ревизия | ONNX | Квантизация |
|--------|---------|------|-------------|
| LLAIMlegal/ru-legal-ner | `924a4b1912ec6e55a4be959cab215ad8ff32a750` | `llaim-ru-legal-ner.onnx` | FP32 |
| redmadrobot-rnd/rubert-base-pii-ner | `c802e8cd26f85d1cf920973ea6f83965a0618d63` | `redmadrobot-rubert-pii-ner-int8.onnx` | dynamic INT8 (QInt8, per_channel, MatMul/Gemm) |

Ревизии и контрольные суммы артефактов зафиксированы в `models/manifest.json`.

### Экспорт моделей

Скрипты экспорта в `scripts/`:

```bash
# Экспорт LLAIM в ONNX FP32
python scripts/export_llaim.py

# Экспорт RedMadRobot в ONNX FP32 + dynamic INT8
python scripts/export_redmadrobot.py

# Проверка, что ONNX-вывод совпадает с PyTorch
python scripts/verify_onnx.py
```

PyTorch используется только для экспорта и проверки корректности.
Основной инференс выполняется через ONNX Runtime (CPU).

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
| `ML_GATE_THRESHOLD` | `0.001` | Порог гейта (эвристический score) |
| `ML_NUM_WORKERS` | `4` | Число обработчиков gRPC |
| `ML_INTRA_OP_THREADS` | `1` | ORT intra-op потоки |
| `ML_INTER_OP_THREADS` | `1` | ORT inter-op потоки |
| `ML_GRPC_PORT` | `50051` | Порт gRPC |
| `ML_MODEL_VERSION` | `1.0.0` | Версия модели в ответе |

## Параллелизм

- Бюджет: 4 обработчика × 1 вычислительный поток, `batch_size=1`.
- ORT: `intra_op=1`, `inter_op=1`, spinning выключен.
- Потоки вычислительных библиотек ограничены до инициализации
  (`ml_service/core/threading_setup.py`).
- Модели загружаются один раз и разделяются между потоками (ORT-сессии
  потокобезопасны).
- Токенизаторы не потокобезопасны (Rust `tokenizers`), поэтому каждый поток
  получает собственный экземпляр через thread-local.

## Тесты

```bash
.venv/bin/python -m pytest
```

## Структура

```
ml-service/
├── ml_service/
│   ├── config.py              # конфигурация
│   ├── core/
│   │   ├── detector.py        # гейт + NER + фильтр
│   │   ├── gate.py            # LLAIM-гейт
│   │   ├── ner.py             # RedMadRobot NER
│   │   ├── windowing.py       # разбиение на окна
│   │   ├── onnx_model.py      # ONNX Runtime обёртка
│   │   ├── labels.py          # маппинги меток
│   │   └── threading_setup.py # ограничение потоков
│   ├── proto/                 # сгенерированный gRPC-код
│   └── server/                # gRPC-сервер
├── models/                    # ONNX-модели и манифест
├── scripts/                   # экспорт, проверка, клиент
└── tests/                     # тесты
```