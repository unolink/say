[English version (README.md)](README.md)

# say

Пакет структурированного логирования на основе Go 1.21+ `slog` с ротацией файлов и hot-reload.

## Возможности

- **Основан на slog** — использует стандартный `log/slog` Go
- **Несколько выводов** — запись в stdout, файлы или оба одновременно
- **Ротация файлов** — автоматическая ротация через lumberjack (размер, возраст, бэкапы)
- **Два формата** — JSON для машин, текст для людей
- **Hot-reload** — изменение уровня, формата и вывода без перезапуска
- **Потокобезопасный** — конкурентное логирование из нескольких goroutines
- **Нулевая конфигурация** — работает из коробки с разумными значениями

## Установка

```bash
go get github.com/unolink/say
```

## Быстрый старт

```go
package main

import "github.com/unolink/say"

func main() {
    // Инициализация с настройками по умолчанию (text, INFO, stdout)
    if err := say.Init(nil); err != nil {
        panic(err)
    }

    say.Info("Application started")
    say.Debug("Debug message", "key", "value")  // Не показывается (уровень INFO)
    say.Error("Something failed", "error", "timeout")
}
```

### Пользовательская конфигурация

```go
compress := true
cfg := &say.Config{
    LevelStr:       "debug",
    Format:         say.FormatJSON,
    OutputStdout:   true,
    OutputFile:     true,
    FilePath:       "/var/log/app.log",
    FileMaxSize:    100,    // МБ
    FileMaxBackups: 5,
    FileMaxAge:     30,     // дней
    FileCompress:   &compress,
}

if err := say.Init(cfg); err != nil {
    panic(err)
}
```

## Параметры конфигурации

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `LevelStr` | string | `"info"` | Уровень: debug, info, warn, error |
| `Format` | Format | `FormatText` | Формат: JSON или text |
| `OutputStdout` | bool | `true` | Вывод в stdout |
| `OutputFile` | bool | `false` | Вывод в файл |
| `FilePath` | string | - | Путь к файлу (обязательно если OutputFile=true) |
| `FileMaxSize` | int | `100` | Макс размер файла в МБ |
| `FileMaxBackups` | int | `3` | Макс количество старых файлов |
| `FileMaxAge` | int | `28` | Макс возраст файлов в днях |
| `FileCompress` | *bool | `true` | Сжимать ротированные файлы |
| `AddSource` | bool | `false` | Добавлять источник в логи |

## Примеры использования

### Структурированное логирование

```go
say.Info("User logged in", "user_id", 123, "ip", "192.168.1.1")

say.Info("API call completed",
    slog.Group("request",
        slog.String("method", "POST"),
        slog.Int("status", 200),
    ),
)
```

### Logger с атрибутами

```go
logger := say.GetLogger().With(
    slog.String("component", "auth"),
    slog.String("version", "1.0.0"),
)

logger.Info("User authenticated", "user", "alice")
// Вывод: ... component=auth version=1.0.0 user=alice
```

### Динамическое изменение уровня

```go
say.Init(nil)
say.Debug("Debug 1")  // Не показывается (уровень по умолчанию INFO)

say.SetLevel("debug")
say.Debug("Debug 2")  // Теперь показывается
```

## Hot-Reload

### Паттерн Observer

Логгеры, созданные через `NewLoggerWithHotReload`, автоматически обновляются при изменении конфигурации:

```go
cfg := &say.Config{
    LevelStr:     "info",
    Format:       say.FormatText,
    OutputStdout: true,
}

logger, err := say.NewLoggerWithHotReload(cfg)
if err != nil {
    panic(err)
}
defer logger.Close() // ВАЖНО: предотвращает утечки памяти

// Позже, при изменении конфигурации:
cfg.LevelStr = "debug"
cfg.Format = say.FormatJSON
cfg.OnUpdate()  // Все подписанные логгеры обновятся
```

### Как это работает

1. `Config.Subscribe()` регистрирует callbacks (паттерн observer)
2. `Config.OnUpdate()` уведомляет всех подписчиков
3. `Logger.Reconfigure()` атомарно заменяет handler
4. Дочерние логгеры (через `With()`) обновляются автоматически через общий `HandlerContainer`

## Зависимости

- `log/slog` — Go stdlib (1.21+)
- `gopkg.in/natefinch/lumberjack.v2` — ротация логов

## Лицензия

MIT — см. [LICENSE](LICENSE).
