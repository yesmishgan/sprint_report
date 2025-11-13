# sprint_report

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)]()
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)]()
[![Build](https://img.shields.io/badge/Build-Passing-brightgreen.svg)]()

CLI-утилита для анализа спринта по выгрузкам из Jira.  
Считает Capacity, Planned, Completed и Delivery для каждого участника.

---

## 📦 Установка

### Установка из репозитория

```bash
go install github.com/yesmishgan/sprint_report@latest
```

### Локальная сборка

```bash
git clone https://github.com/yesmishgan/sprint_report.git
cd sprint_report
go build -o sprint_report ./...
```

### Готовый бинарник

Также можно воспользоваться готовым бинарным файлом, если он у вас запустится ✨

---

## ⚙️ Конфигурация команды

Для корректной работы команды необходимо подготовить файл с конфигурацией команды, в котором будут описаны члены команды
и их капаситет в рамках спринта `team_config.json`:

```json
{
  "team": "backend-core",
  "members": [
    {
      "login": "mike",
      "capacity": 9
    },
    {
      "login": "olga",
      "capacity": 9
    },
    {
      "login": "ivan",
      "capacity": 6
    }
  ]
}
```

---

## ▶️ Использование

### Формирование точки отсчета спринта (init)

```bash
sprint_report -cmd init   -csv sprint_start.csv   -config team_config.json   -state sprint_state.json   -sprint "Sprint 42 (2025-11-03 — 2025-11-16)"
```

### Построение отчёта (report)

```bash
sprint_report -cmd report   -csv sprint_end.csv   -state sprint_state.json   -done-statuses "Done,Closed,Resolved"   -format table
```

---

## 🧩 Примеры CSV-выгрузок

Выгрузка CSV с информацией о задачах спринта должна содержать следующие обязательные поля из JIT:

1. Assignee
2. Issue key
3. Story Points (системное название в выгрузке ```Custom field (Story Points)```)
4. Responsible QA (системное название в выгрузке ```Custom field (Responsible QA))```)
5. QA Estimate (системное название в выгрузке ```Custom field (QA Estimate)```)
6. Status
---

## 📊 Примеры вывода

### Табличный вывод

```
Name              Capacity   Planned   Completed   Delivery
Team (backend)    24.00      43.00     39.00       91%
mike              9.00       12.00     15.00       125%
olga              9.00       16.00     13.00       81%
ivan              6.00       15.00     11.00       73%
```

### JSON вывод

```json
{
  "sprint_name": "Sprint 42",
  "team": {
    "name": "Team (backend-core)",
    "capacity": 24,
    "planned": 43,
    "completed": 39,
    "delivery": 91
  },
  "users": [
    {
      "name": "mike",
      "capacity": 9,
      "planned": 12,
      "completed": 15,
      "delivery": 125
    }
  ]
}
```

---

## 🧮 Логика расчётов

### Planned

```
Planned = Σ Story Points (Assignee) + Σ QA Estimate (Responsible QA)
```

### Completed

Учитывает и суммирует:

1. SP задач, попавших в done-статусы
2. QA Estimate задач в done
3. Разницу в SP для частично выполнённых задач:

### Delivery

```
Delivery = (Completed / Planned) * 100
```

---

## 🔄 Типовой рабочий процесс

В начале спринта:

1. Делаем выгрузку задач, попавших в спринт команды
2. Выполняем инициализацию спринта с указанием аргументов утилиты

```bash
sprint_report -cmd init -csv sprint_start.csv -config team_config.json -state sprint_state.json
```

3. Сохраняем промежуточный стейт спринта для последующего подсчета итоговой статистики по итогам прошедшего спринта

В конце спринта:

1. Делаем выгрузку задач из спринта
2. Выполняем формирование отчета спринта с указанием аргументов утилиты

```bash
sprint_report -cmd report -csv sprint_end.csv -state sprint_state.json -format table
```

---

## 📜 License

MIT License
