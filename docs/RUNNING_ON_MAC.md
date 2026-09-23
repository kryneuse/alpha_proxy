# Запущенная версия на этом Mac

Дата: 23 сентября 2026. Сервис оставлен запущенным в фоне.

- Репозиторий: `/Users/aekuleshevskii/alpha_proxy`.
- HTTP: `http://127.0.0.1:8080`, единственная прикладная ручка `POST /process`.
- 6 отдельных Python ML-процессов; в каждом 1 quality worker и 2 spaCy workers.
- Quality: gate1-v3 на residual, затем v14a (~99M) на оригинале при положительном гейте. Fast: spaCy sm на оригинале без гейта. Правила сохраняются в обоих случаях.
- ML gRPC: `127.0.0.1:50051`–`50056`, round_robin из Go.
- ML metrics: `http://127.0.0.1:9091/metrics`–`9096/metrics`; `/stats` возвращает JSON.
- Компьютер: Mac16,7, 14 CPU-ядер, 48 GiB RAM. Используется CPU; GPU-экспорт не выполнялся.
- Отдельное окружение: `.venv-adaptive` (Python3.12.13). Runtime-зависимости перенесены из проверенного окружения на этом же Mac; инвентарь в `.venv-adaptive/requirements.installed.txt`, provenance рядом. Установка пакетов из сети не была условием запуска.
- API-key авторизация включена; ключ находится только в `.runtime/adaptive/systems.json` (0600).

## Проверить маскирование и демаскирование

```bash
cd /Users/aekuleshevskii/alpha_proxy
.venv-adaptive/bin/python scripts/demo_local.py
```

Скрипт сам читает локальный ключ, вызывает настоящий HTTP API и проверяет маскирование, идемпотентный retry и точное восстановление исходной строки с emoji/кириллицей.

Для собственных вызовов передайте `Content-Type: application/json` и `X-API-Key`, JSON вида:

```json
{"payload_id":"my-unique-id","payload":"Иван Петров, email example@example.com"}
```

Новый payload_id означает маскирование. Для демаскирования отправьте **тот же payload_id**, а в payload — result первого ответа с токенами. Повтор исходного payload с тем же ID возвращает прежний замаскированный результат.

Mappings хранятся в памяти Go 15 минут; после перезапуска демаскировать старые сессии нельзя. Локальный лимит — 1 млн сессий. Переключение ML backend или реплик mappings не меняет.

## Запуск и остановка

Текущий процесс уже запущен; повторный запуск на занятых портах завершается с ошибкой и не трогает существующий сервис.

```bash
# Запустить в переднем плане; Ctrl-C корректно завершает собственные процессы.
.venv-adaptive/bin/python scripts/run_local_adaptive.py
```

Для фонового запуска после остановки:

```bash
nohup .venv-adaptive/bin/python -u scripts/run_local_adaptive.py > .runtime/adaptive/supervisor.log 2>&1 &
```

PID супервизора и дочерних процессов записаны в `.runtime/adaptive/run.json`; PID супервизора также находится в `.runtime/adaptive/supervisor.pid`. Для остановки отправьте SIGTERM именно этому супервизору. Логи: `.runtime/adaptive/http.log`, `ml-0.log`–`ml-5.log`, `supervisor.log`.

## Финальные измерения HTTP

Все значения получены на запущенной версии с авторизацией, правилами, нейросетями, gRPC, сохранением mappings и JSON. Использовался фиксированный вымышленный текст, новые payload_id, без кеширования retry. Это короткий локальный нагрузочный тест; не независимая оценка качества и не обещание RPS на произвольных текстах.

| Сценарий | Успешных операций/с | HTTP запросов/с | p95 операции | Ошибки |
|---|---:|---:|---:|---:|
| Маскирование, 250 code points, concurrency96, 15s | 2565.8 | 2565.8 | 93.5ms | 0 |
| Маскирование, 1000 code points, concurrency48, 12s | 664.1 | 664.1 | 130.2ms | 0 |
| Маскирование + демаскирование, 250 code points, concurrency48, 10s | 2338.9 пар/с | 4677.7 | 51.2ms на пару | 0 |

В последнем сценарии все 23533 пары восстановились побайтово точно. При нагрузке spaCy обработала примерно97% чанков; это режим сниженного качества. Перед нагрузкой и после нее 12 из12 последовательных HTTP-запросов маршрутизировались в основной quality-каскад.

Среди трех коротко проверенных CPU-профилей выбран 6×(1 quality+2 spaCy). Это лучший измеренный профиль, не доказанный абсолютный аппаратный максимум. Сравнение: `adaptive_profile_results.json`; полный финальный отчет: `adaptive_validation_mac.json`.

## Проверки

- `go test ./...`: прошел.
- Race: internal/ml, internal/store, internal/masking, internal/tokenizer — прошел.
- Python в установленном окружении: 40 passed, 2 skipped (необязательные старые LLAIM/transformers window tests).
- Проверены очереди, hysteresis, восстановление после устаревшего EWMA, ошибки/отмена, исходный контекст NER, Unicode и все25 transport types, совпадение spaCy с эталонными fixtures, живой gRPC и реальный HTTP round trip.

Подробности архитектуры, настройки и команды бенчмарка: [ADAPTIVE.md](ADAPTIVE.md).
