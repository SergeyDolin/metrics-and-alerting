# Оптимизация памяти агента системы мониторинга

## Результаты профилирования и оптимизации

### 1. Базовый профиль (до оптимизаций)

```bash
$ go tool pprof -top profiles/base.pprof
File: agent
Type: inuse_space
Time: 2026-02-25 13:33:00 +07
Showing nodes accounting for 4490.73kB, 100% of 4490.73kB total
      flat  flat%   sum%        cum   cum%
    2052kB 45.69% 45.69%     2052kB 45.69%  runtime.allocm
  902.59kB 20.10% 65.79%   902.59kB 20.10%  compress/flate.NewWriter (inline)
  512.05kB 11.40% 77.20%   512.05kB 11.40%  net/http.(*Transport).getConn
  512.05kB 11.40% 88.60%   512.05kB 11.40%  runtime.(*scavengerState).init
  512.05kB 11.40%   100%   512.05kB 11.40%  runtime.acquireSudog
         0     0%   100%   902.59kB 20.10%  compress/gzip.(*Writer).Write
         0     0%   100%  1414.63kB 31.50%  main.sendRequest
         0     0%   100%  1414.63kB 31.50%  main.sendMetricJSON
```

### 2. Профиль после оптимизаций

```bash
$ go tool pprof -top profiles/result.pprof
File: agent
Type: inuse_space
Time: 2026-02-25 13:35:34 +07
Showing nodes accounting for 3092.21kB, 100% of 3092.21kB total
      flat  flat%   sum%        cum   cum%
    2052kB 66.36% 66.36%     2052kB 66.36%  runtime.allocm
  528.17kB 17.08% 83.44%   528.17kB 17.08%  net/http.init.func15
  512.05kB 16.56%   100%   512.05kB 16.56%  runtime.acquireSudog
         0     0%   100%   528.17kB 17.08%  sync.(*Pool).Get
```

### 3. Сравнительный анализ

```bash
$ go tool pprof -top -diff_base=profiles/base.pprof profiles/result.pprof
File: agent
Type: inuse_space
Time: 2026-02-25 13:33:00 +07
Showing nodes accounting for -1398.51kB, 31.14% of 4490.73kB total
      flat  flat%   sum%        cum   cum%
 -902.59kB 20.10% 20.10%  -902.59kB 20.10%  compress/flate.NewWriter (inline) 
  528.17kB 11.76%  8.34%   528.17kB 11.76%  net/http.init.func15
 -512.05kB 11.40% 19.74%  -512.05kB 11.40%  net/http.(*Transport).getConn 
 -512.05kB 11.40% 31.14%  -512.05kB 11.40%  runtime.(*scavengerState).init
         0     0% 31.14%  -902.59kB 20.10%  compress/gzip.(*Writer).Write 
         0     0% 31.14% -1414.63kB 31.50%  main.sendRequest 
         0     0% 31.14% -1414.63kB 31.50%  main.sendMetricJSON 
```

## Результаты оптимизации

### Ключевые улучшения

| Функция | Изменение | Процент | Описание |
|---------|-----------|---------|----------|
| compress/flate.NewWriter | -902.59 kB | -20.10% | Устранение аллокаций при сжатии |
| main.sendRequest | -1414.63 kB | -31.50% | Оптимизация отправки запросов |
| main.sendMetricJSON | -1414.63 kB | -31.50% | Оптимизация сериализации метрик |
| net/http.(*Transport).getConn | -512.05 kB | -11.40% | Улучшение управления соединениями |
| compress/gzip.(*Writer).Write | -902.59 kB | -20.10% | Оптимизация записи сжатых данных |

### Итоговые метрики

| Метрика | До | После | Изменение |
|---------|-----|-------|-----------|
| Общее использование памяти | 4490.73 kB | 3092.21 kB | **-31.14%** |
| Аллокации в сжатии данных | 902.59 kB | 0 kB | -100% |
| Аллокации в отправке запросов | 1414.63 kB | 0 kB | -100% |

## Бенчмарки

### До оптимизаций

```bash
BenchmarkSendMetricJSON-10          6489   161492 ns/op   841292 B/op   106 allocs/op
BenchmarkSendBatchJSON-10           7609   177082 ns/op   841273 B/op   109 allocs/op
BenchmarkCollectRuntimeMetrics-10  53704    22876 ns/op     7080 B/op     4 allocs/op
BenchmarkMemStorageUpdateGauge-10   55.9M     21.22 ns/op      0 B/op     0 allocs/op
BenchmarkMemStorageGetAll-10        16.1M     72.37 ns/op     72 B/op     2 allocs/op
```

### После оптимизаций

```bash
BenchmarkSendMetricJSON-10          7428   155151 ns/op   841287 B/op   103 allocs/op
BenchmarkSendBatchJSON-10           7911   160162 ns/op   841678 B/op   106 allocs/op
BenchmarkCollectRuntimeMetrics-10  53767    22096 ns/op     7080 B/op     4 allocs/op
BenchmarkMemStorageUpdateGauge-10   56.3M     21.57 ns/op      0 B/op     0 allocs/op
BenchmarkMemStorageGetAll-10        15.5M     74.53 ns/op     72 B/op     2 allocs/op
```

### Сравнение бенчмарков

| Бенчмарк | До (ns/op) | После (ns/op) | Изменение | Аллокации до | Аллокации после |
|----------|------------|---------------|-----------|--------------|-----------------|
| SendMetricJSON | 161,492 | 155,151 | **-3.9%** | 106 | 103 |
| SendBatchJSON | 177,082 | 160,162 | **-9.6%** | 109 | 106 |
| CollectRuntimeMetrics | 22,876 | 22,096 | **-3.4%** | 4 | 4 |
