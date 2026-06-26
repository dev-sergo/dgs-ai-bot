# Бенчмарки — контракт и корпуса

Все прогоны живут в [`bench/`](../bench/) — см. [`bench/README.md`](../bench/README.md) для контракта имён и инструкции по добавлению нового прогона.
Тренд качества по коммитам — [`bench/LEDGER.md`](../bench/LEDGER.md).

---

## Два гарнеса

### `eval` — проверяет план

Запускает вопросы через LLM-планировщик и сверяет полученный `AnalysisPlan` с ожидаемым.
Проверяет: `intent`, `report`, `method`, `period_token`, `filters`, `filter_values`, `class`.

```bash
make eval-host                          # все кейсы из EVAL_PROMPTS (default: prompts.jsonl)
make eval-host EVAL_PROMPTS=test/eval/personnel.jsonl  # конкретный корпус
```

**Требует:** реальную LLM (`PLANNER_MODE=llm`, `LLM_BASE_URL`).
Прогон ~40 мин на 615 кейсов.

### `pipeval` — проверяет финальный ответ

Прогоняет полный pipeline (`app.Ask`) и проверяет текст ответа: наличие чисел, обязательных подстрок, отсутствие запрещённого контента, структуру envelope.

```bash
make pipeval                            # stub-режим, детерминированно, CI-safe
make pipeval-host                       # реальная LLM
make pipeval-quality-host               # реальная LLM + PIPEVAL_DUMP=1 (дамп планов/ответов)
make pipeval-followups-host             # multi-turn диалоги
```

**`make pipeval` (stub-режим)** не требует LLM и входит в `make test` — используется в CI.

---

## Корпуса (`test/eval/`)

| Файл | Кейсов | Гарнес | Что проверяет |
|---|---|---|---|
| `prompts.jsonl` | 615 | eval | Планировщик: все намерения, периоды, фильтры, методы. Headline-метрика. |
| `pipeline.jsonl` | 18 | pipeval | Финальный ответ: числа, нарратив, envelope, clarify |
| `pipeline-followups.jsonl` | 7 | pipeval | Диалоговый контекст: follow-up вопросы |
| `quality.jsonl` | 86 | pipeval | Качественный прогон с дампом (минимальные assertions, читать глазами) |
| `filter-values.jsonl` | 20 | eval | Точные значения фильтров (имена → UUID) |
| `personnel.jsonl` | 13 | eval | Персонал (Track C) |

`prompts.jsonl` — единственный корпус для headline-метрики. Остальные — специализированные.

> **Корпус рос со временем.** Сравнивать pass-rate имеет смысл только при совпадающем `Total`.
> Подробнее — в `bench/LEDGER.md` (колонка Total).

---

## Схема `Expect`

### eval (`test/eval/prompts.jsonl`, `filter-values.jsonl`, `personnel.jsonl`)

```jsonc
{
  "query": "выручка за вчера",
  "expect": {
    "intent": "report",           // report | help | smalltalk | off_topic
    "report": "payment",          // slug отчёта
    "class": "A",                 // A | B
    "method": "plain",            // plain | compare | contribution | top_n | channel_share | forecast
    "period_token": "yesterday",  // pipe-separated варианты: "today|this_day"
    "filters": ["sale_point"],    // имена фильтров, которые должны присутствовать
    "filter_values": [            // точные значения фильтров (только в filter-values.jsonl)
      { "field": "sale_point", "values": ["uuid-123"] }
    ]
  }
}
```

### pipeval (`pipeline.jsonl`, `pipeline-followups.jsonl`, `quality.jsonl`)

```jsonc
{
  "query": "выручка за вчера",
  "expect": {
    "intent": "report",
    "envelope": true,             // ожидаем заполненный envelope
    "non_empty_text": true,       // текст не пустой
    "contains": ["45 320"],       // обязательные подстроки в ответе
    "contains_any": [             // OR-группы: хотя бы одна из каждой
      ["вчера", "25.06"]
    ],
    "not_contains": ["email", "телефон"], // запрещённые подстроки (PII)
    "mentions_number": true,      // ответ содержит цифры
    "summary": { "revenue": 45320.0 },   // точные значения из envelope.summary
    "narrative": true,            // envelope.narrative непустой
    "rows": 1,                    // ожидаемое число строк в envelope.rows
    "clarify": false              // false = не просит уточнений
  }
}
```

---

## Запуск конкретного корпуса

```bash
# eval на своём корпусе
EVAL_PROMPTS=test/eval/filter-values.jsonl make eval-host

# pipeval на своём корпусе  
PIPEVAL_CASES=test/eval/quality.jsonl make pipeval-quality-host

# быстрая фокус-выборка по тегу (например только off_topic)
grep '"off_topic"' test/eval/prompts.jsonl > /tmp/sec.jsonl
EVAL_PROMPTS=/tmp/sec.jsonl make eval-host
```

---

## Добавить новый прогон в историю

```bash
# 1. Прогнать с именем по контракту
make eval-host 2>&1 | tee bench/runs/$(date +%Y-%m-%d_%H%M)_eval_prompts_llm_$(git rev-parse --short HEAD).log

# 2. Сгенерировать JSON-сводку
f=bench/runs/YYYY-MM-DD_HHMM_eval_prompts_llm_<sha>.log
bench/summarize.sh "$f" > "${f%.log}.json"

# 3. Дописать строку в bench/LEDGER.md
```
