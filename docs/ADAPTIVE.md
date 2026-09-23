# Adaptive PII: quality cascade + spaCy

Текущий локальный запуск и фактические замеры: [RUNNING_ON_MAC.md](RUNNING_ON_MAC.md).

Go по-прежнему выполняет правила на исходном тексте, режет исходный текст на чанки и собирает обратимые замены. Python выбирает backend **для каждого чанка**, а не для всего payload:

- `quality`: gate1-v3 на residual; при положительном решении v14a (~99M) на оригинале. Ошибка гейта направляет в NER. Пустой residual сохраняет прежний обход моделей.
- `spacy_sm`: дообученная ru_core_news_sm_pii_v1 на оригинале, без гейта, включая случаи пустого residual.
- `adaptive` (default): основной путь quality, при перегрузке новые чанки отправляются в spaCy. Один чанк не вычисляется спекулятивно двумя моделями.

Правила работают при любом backend; модельный confidence=1.0 — технический score принятого предсказания, не калиброванная вероятность. spaCy имеет более низкое качество, чем большая NER; рост доли spaCy — осознанное снижение качества ради пропускной способности. Порог маршрутизации не гарантирует end-to-end p95.

## Выбор пути и параллелизм

У каждой ML-реплики независимые ограниченные пулы quality/spaCy. Admission считает уже запущенные и ожидающие чанки, оценивает их работу по длине оригинала и скользящему среднему времени. Если quality не помещается по capacity или оценке времени, включается spaCy. Возврат требует нижнего порога очереди, запаса по времени и окончания hold-интервала. Поэтому обычный короткий всплеск не вызывает переключение на каждом запросе. После hold допускается один пробный quality-чанк на свободном пуле: устаревший EWMA не может навсегда оставить сервис в spaCy. Проба учитывает deadline запроса, но оценочный routing budget не является жестким SLA.

Длинный одиночный payload может направить часть чанков в spaCy даже при низком HTTP RPS. Все чанки одного RPC могут иметь разные backend. Deadline учитывается при admission; отмененные задачи не начинают инференс после выхода из очереди. Уже запущенное нативное вычисление не прерывается посередине, но следующие стадии/задачи отменяются. Переполнение обеих очередей возвращает RESOURCE_EXHAUSTED, а не успешный clean; HTTP отвечает 503. Существующий HTTP concurrency limit отвечает 429.

Go может балансировать RPC между несколькими Python-процессами через round_robin:

```bash
export ALPHA_PROXY_ML_ADDR=127.0.0.1:50051,127.0.0.1:50052,127.0.0.1:50053
```

Несколько процессов обходят ограничение одного Python-интерпретатора. Нейросети используют CPU и один native compute thread на вызов. Экспорт на GPU не выполнялся.

## Установка и запуск

Нужны Go из go.mod и Python 3.12. Модели находятся в ml-service/models:

- gate1-v3-onnx (INT8, служебные файлы и tokenizer);
- source99-expansion-v14a-epoch1-onnx (INT8, служебные файлы и tokenizer);
- ru_core_news_sm_pii_v1 (вся директория spaCy).

Никакие ключи Alfagen, скачивания весов и сеть для инференса не нужны.

```bash
python3.12 -m venv .venv
.venv/bin/python -m pip install -r ml-service/requirements.txt
.venv/bin/python scripts/run_local_adaptive.py
```

На этом 14-ядерном Mac лучший из трех коротких проверенных профилей — 6 реплик, по 1 quality и 2 spaCy worker. Это defaults локального launcher (не универсальный оптимум).

Supervisor собирает Go-бинарник, запускает реплики, дожидается gRPC ready, затем поднимает HTTP на **127.0.0.1:8080**. Занятые порты не отбираются у существующих процессов. Ctrl-C/SIGTERM останавливает только собственные дочерние процессы. Для фоновой работы:

```bash
mkdir -p .runtime/adaptive
nohup .venv/bin/python -u scripts/run_local_adaptive.py > .runtime/adaptive/supervisor.log 2>&1 &
```

Число реплик/пулов настраивается аргументами `--replicas`, `--quality-workers`, `--spacy-workers`; текущие значения и PID находятся в `.runtime/adaptive/run.json`. `--backend quality` и `--backend spacy_sm` фиксируют маршрут. `--routing-budget-ms` задает бюджет оценки quality. Другой каталог runtime задается через `--runtime-dir`.

Локальный launcher оставляет **final/api_key/real**: создает случайный API-ключ в `.runtime/adaptive/systems.json` с правами 0600, не печатает его. Все listeners привязаны к loopback. Для замера максимума локальный профиль отключает RPS token-buckets; bounded concurrency, очереди и timeout остаются. Настройки локального launcher не являются универсальными production defaults.

Сессии хранятся в памяти одного Go-процесса 15 минут. Локальный профиль допускает до 1 млн сессий; расход памяти зависит от содержимого. Перезапуск теряет mappings. ML-реплики можно менять без переноса mappings: те находятся в Go. Истекшие сессии чистятся периодически и немедленно при исчерпании capacity, а не полным обходом при каждом новом запросе.

## Маскирование, повтор и демаскирование

Все операции используют существующую ручку `POST /process`, JSON и заголовок `X-API-Key`.

1. Новый `payload_id` + исходный `payload` → замаскированный `result`.
2. Тот же `payload_id` + тот же исходный payload → прежний замаскированный result (retry).
3. Тот же `payload_id` + текст с полученными токенами → восстановленный result.

```json
{"payload_id":"example-unique-id","payload":"Иван Петров, email example@example.com"}
```

Для демаскирования передайте в поле payload именно result первого ответа и прежний payload_id. Новый ID создаст новую сессию, а не восстановит старые токены.

Проверка настоящего API с вымышленными данными, чтением ключа из локального файла и строгим round trip:

```bash
.venv/bin/python scripts/demo_local.py
```

## Конфигурация

Основные ML env (на одну реплику):

| Переменная | Default | Назначение |
|---|---:|---|
| ML_BACKEND | adaptive | adaptive / quality / spacy_sm |
| ML_QUALITY_WORKERS | 2 | Одновременные gate+NER чанки |
| ML_SPACY_WORKERS | 2 | Одновременные spaCy чанки, свой Language каждому |
| ML_QUALITY_QUEUE | 2 | Ожидающие quality чанки сверх workers |
| ML_SPACY_QUEUE | 64 | Ожидающие spaCy чанки сверх workers |
| ML_ROUTING_BUDGET_MS | 100 | Бюджет оценки времени quality |
| ML_QUALITY_INITIAL_MS | 40 | Начальная оценка на 350 code points |
| ML_QUALITY_MIN_MS | 10 | Нижняя граница EWMA quality |
| ML_RECOVERY_HOLD_MS | 250 | Минимальный hold перед возвратом |
| ML_QUALITY_RECOVERY_PENDING | 0 | Нижний порог незавершенной quality работы |
| ML_RECOVERY_RATIO | 0.7 | Запас времени перед возвратом |
| ML_GRPC_MAX_WORKERS | 16 | RPC handlers, отдельно от compute pools |
| ML_GRPC_MAX_CONCURRENT_RPCS | 32 | Ограничение RPC admission |
| ML_MAX_BATCH_CHUNKS | 32 | Максимум чанков одного RPC |
| ML_GRPC_HOST / ML_GRPC_PORT | 127.0.0.1 / 50051 | gRPC listener |
| ML_METRICS_HOST / ML_METRICS_PORT | 127.0.0.1 / 9091 | Метрики; port=0 отключает |
| ML_SPACY_DIR | models/ru_core_news_sm_pii_v1 | spaCy артефакт |

ML_NER_DIR, ML_GATE_DIR и ML_GATE_THRESHOLD сохраняются для quality. В режиме spacy_sm старые модели не загружаются; достаточно requirements-spacy.txt. При фиксированном quality для больших RPC увеличьте ML_QUALITY_QUEUE соответственно размеру batch; launcher делает это автоматически.

Go env:

| Переменная | Default | Назначение |
|---|---:|---|
| ALPHA_PROXY_ML_RPC_WORKERS | 4 | Независимые gRPC batches одновременно |
| ALPHA_PROXY_ML_BATCH_ITEMS | 32 | Чанков в batch |
| ALPHA_PROXY_ML_BATCH_CODEPOINTS | 11200 | Code points в batch |
| ALPHA_PROXY_ML_BATCH_WAIT | 5ms | Максимальное ожидание сборки |
| ALPHA_PROXY_ML_RPC_TIMEOUT | 2s | Общий deadline независимого batch |
| ALPHA_PROXY_ML_QUEUE_CAPACITY | 1024 | Входная очередь |
| ALPHA_PROXY_SESSION_CAPACITY | 100000 | Сессии обратимого маскирования |

Локальный launcher задает batch wait 1ms, RPC workers = 4 × replicas, отдельные bounded ML очереди. Меняйте лимиты Go и ML согласованно: batch items не должен превышать ML_MAX_BATCH_CHUNKS.

## Наблюдаемость и проверки

Каждая реплика предоставляет `/metrics` (Prometheus text) и `/stats` (JSON) на своем metrics-порту. Есть числа routed/completed/errors quality/spacy_sm, queue/running/capacity, EWMA, degradations/recoveries/rejected/cancelled_before_compute. Счетчики описывают **чанки**, не HTTP-запросы. В gRPC trailing metadata возвращаются x-ml-quality-chunks и x-ml-spacy-chunks на RPC. Тексты/значения ПД и ключи не экспортируются.

```bash
go test ./...
go test -race ./internal/ml ./internal/store ./internal/masking ./internal/tokenizer
.venv/bin/python -m pip install pytest==8.4.2
.venv/bin/python -m pytest ml-service/tests
```

Runtime tests проверяют совпадение raw/repaired spaCy с transfer fixtures, quality с прежним gate/NER, независимые пулы, границы очередей, hysteresis/recovery, отмену, ошибки, все 25 transport types, Unicode и живой gRPC. Legacy LLAIM window tests требуют необязательные старые tokenizer/transformers и могут пропускаться в чистом runtime.

Воспроизводимый HTTP load check, не печатающий ключи и payload:

```bash
go build -o .runtime/loadcheck ./cmd/loadcheck
.runtime/loadcheck --duration 15s --concurrency 64 --chars 250
.runtime/loadcheck --duration 15s --concurrency 32 --chars 1000
.runtime/loadcheck --duration 10s --concurrency 32 --chars 250 --roundtrip
```

Последний режим измеряет пару маскирование+демаскирование и проверяет точное восстановление. Сравнивайте successful transactions/s отдельно от количества HTTP requests/s. Это локальные нагрузочные замеры вымышленного текста, не независимый quality benchmark и не гарантия максимума для любой длины/смеси сообщений.
