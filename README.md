# call-tester

Система тестирования голосовых вызовов, SMS и мобильного интернета для верификации биллинга операторов. Go, Raspberry Pi 5, модемы Quectel EC25-EUX.

## Возможности

- **Голос** — звонки модем-на-модем с замером времени набора, ответа, длительности разговора
- **SMS** — отправка между модемами с замером delivery time, мультипарт (>160 символов), на внешние номера, приём
- **Интернет** — QMI-сессии с policy routing, замер трафика (байт RX/TX), http_get, ping через конкретный интерфейс
- **Сценарии** — YAML-файлы, последовательные и параллельные блоки, tolerant execution (ошибка одного шага не останавливает остальные)
- **Отчёты** — JSON (полные данные) + 3 CSV (calls/sms/data) для сверки с CDR оператора
- **Prometheus-метрики** — uptime модемов, сигнал, счётчики звонков/SMS/байтов на `:9100/metrics`
- **Docker** — готовый образ с пробросом USB-устройств, multi-stage сборка под ARM64
- **Кросс-компиляция** — одна команда `make deploy` собирает на маке и заливает на RPi

## Архитектура

```
call-tester/
├── Makefile                           # Сборка + деплой + Docker
├── Dockerfile / docker-compose.yml
├── config.yaml                        # Конфигурация модемов и метрик
├── setup_rpi.sh                       # Подготовка RPi
├── scenarios/
│   └── full_test.yaml                 # Сценарии тестирования
├── reports/                           # Сюда пишутся отчёты
├── cmd/
│   └── call-tester/
│       └── main.go                    # CLI — точка входа
└── internal/
    ├── modem/
    │   └── modem.go                   # AT-команды, голос, SMS
    ├── models/
    │   └── models.go                  # Конфиг, сценарии, CDR
    ├── engine/
    │   └── engine.go                  # Движок сценариев, параллельность
    ├── data/
    │   ├── qmi.go                     # QMI-сессии, DHCP, policy routing
    │   └── actions.go                 # http_get, ping, idle
    ├── metrics/
    │   └── metrics.go                 # Prometheus + HTTP /metrics
    └── report/
        └── report.go                  # CSV/JSON отчёты
```

Параллельность реализована через goroutines + `sync.WaitGroup`. Каждый модем — обычная структура с serial-портом, координация через каналы в engine. При ошибке одного шага — логируется, остальные продолжаются.

## Быстрый старт

### 1. Подготовка RPi

```bash
ssh pi@192.168.0.107

# Зависимости
sudo apt update
sudo apt install -y libqmi-utils udhcpc curl iproute2 iputils-ping

# ModemManager захватывает AT-порты — отключаем обязательно
sudo systemctl stop ModemManager
sudo systemctl disable ModemManager

# Группа dialout для доступа к /dev/ttyUSB*
sudo usermod -aG dialout $USER
# Перелогиниться!
```

Или одной командой: `chmod +x setup_rpi.sh && ./setup_rpi.sh`

### 2. Сборка и деплой (с мака)

```bash
# Установить Go если нет
brew install go

cd call-tester
go mod tidy           # скачать зависимости
make deploy           # собрать под ARM64, залить на RPi (~3 секунды)
```

### 3. Сборка прямо на RPi (альтернатива)

```bash
sudo apt install -y golang
cd call-tester
go mod tidy
go build -o call-tester ./cmd/call-tester
```

### 4. Настройка и запуск

```bash
# На RPi
cd ~/call-tester

# Отредактировать конфиг — вписать реальные порты и номера SIM
nano config.yaml

# Проверить все модемы
./call-tester check

# Запустить полный сценарий
./call-tester run scenarios/full_test.yaml
```

## Конфигурация

Формат YAML. Пример:

```yaml
metrics:
  enabled: true
  listen: "0.0.0.0:9100"

modems:
  - name: ec25_1
    at_port: /dev/ttyUSB11       # AT-порт (интерфейс 02 у Quectel)
    qmi_device: /dev/cdc-wdm0    # QMI-устройство
    net_iface: wwan0             # WWAN-интерфейс
    baud_rate: 115200
    model: Quectel EC25-EUX
    phone_number: "+77001111111"   # Реальный номер SIM
    apn: internet
    apn_user: ""
    apn_pass: ""
    operator: Tele2
```

### Определение портов модемов

```bash
# AT-порты (у Quectel интерфейс 02)
for port in /dev/ttyUSB*; do
  iface=$(udevadm info -a "$port" 2>/dev/null | grep 'ATTRS{bInterfaceNumber}' | head -1 | awk -F'"' '{print $2}')
  vendor=$(udevadm info -a "$port" 2>/dev/null | grep 'ATTRS{idVendor}' | head -1 | awk -F'"' '{print $2}')
  echo "$port -> vendor=$vendor iface=$iface"
done

# QMI-устройства
ls /dev/cdc-wdm*

# WWAN-интерфейсы
ls /sys/class/net/ | grep wwan
```

## Команды CLI

```bash
# Проверить все модемы (сигнал, регистрация в сети)
./call-tester check

# Запустить сценарий
./call-tester run scenarios/full_test.yaml

# С кастомной директорией отчётов
./call-tester run scenarios/full_test.yaml -o /var/log/reports/

# С другим конфигом
./call-tester -c /etc/call-tester/config.yaml run scenarios/full_test.yaml

# Показать пример конфигурации
./call-tester example-config

# Показать пример сценария
./call-tester example-scenario
```

## Формат сценариев

YAML-файлы. Поддерживаются 6 типов шагов, все можно миксовать.

### Типы шагов

| Шаг | Параметры | Описание |
|-----|-----------|----------|
| `call` | `from_modem`, `to_modem`, `hold_duration_sec` | Голосовой вызов |
| `sms_send` | `from_modem`, `to_modem` или `to_number`, `text` | Отправка SMS |
| `sms_wait` | `from_modem`, `timeout_sec` | Ожидание входящего SMS |
| `data_session` | `modem`, `actions[]` | Интернет-сессия через QMI |
| `pause` | `duration_sec` | Пауза между шагами |
| `parallel` | `steps[]` (вложенные шаги) | Параллельное исполнение |

### Действия внутри data_session

| Тип | Параметры | Описание |
|-----|-----------|----------|
| `http_get` | `url`, `timeout_sec` | Скачать URL через curl |
| `ping` | `host`, `count` | Серия пингов через интерфейс |
| `idle` | `duration_sec` | Держать сессию открытой |

### Пример комбинированного сценария

```yaml
name: billing_check
description: Полный тест для сверки тарификации

steps:
  # Звонок
  - action: call
    from_modem: ec25_1
    to_modem: ec25_2
    hold_duration_sec: 30

  - action: pause
    duration_sec: 5

  # SMS модем-на-модем (с замером delivery time)
  - action: sms_send
    from_modem: ec25_1
    to_modem: ec25_2
    text: "Test billing SMS"

  - action: pause
    duration_sec: 5

  # Параллельно: два модема в интернете одновременно
  - action: parallel
    steps:
      - action: data_session
        modem: ec25_1
        actions:
          - type: http_get
            url: "https://httpbin.org/bytes/1048576"
          - type: ping
            host: "8.8.8.8"
            count: 10

      - action: data_session
        modem: ec25_2
        actions:
          - type: http_get
            url: "https://httpbin.org/bytes/524288"
```

## Формат отчётов

После каждого запуска в `reports/` появятся файлы:

```
report_billing_check_20260416_143012.json   # Полные данные
calls_billing_check_20260416_143012.csv     # CDR звонков
sms_billing_check_20260416_143012.csv       # CDR SMS
data_billing_check_20260416_143012.csv      # CDR data-сессий
```

### CSV звонков (для сверки с биллингом голоса)

| Поле | Описание |
|------|----------|
| Номер_А / Номер_Б | Участники вызова |
| Время_начала | Момент набора номера |
| Время_ответа | Момент соединения (биллинг тарифицирует с этого момента) |
| Время_конца | Момент завершения |
| Длительность_разговора_сек | Для сравнения с тарифицированной длительностью |

### CSV SMS (для сверки с биллингом SMS)

| Поле | Описание |
|------|----------|
| Номер_А / Номер_Б | Отправитель / получатель |
| Время_отправки / Время_получения | Тайминги |
| Частей_SMS | Сколько биллинговых единиц (1 если ≤160 символов GSM-7) |
| Время_доставки_сек | Задержка доставки (при модем-на-модем) |

### CSV data (для сверки с биллингом трафика)

| Поле | Описание |
|------|----------|
| Номер / Оператор / APN | Идентификация сессии |
| Байт_RX / Байт_TX / МБ_всего | Объём трафика |
| Длительность_сек | Длительность сессии |
| IP_адрес | Выделенный IP |

## Prometheus метрики

На `http://RPI_IP:9100/metrics` доступны метрики:

```
# Модемы
modem_registered{modem="ec25_1",operator="Tele2"} 1
modem_signal_rssi{modem="ec25_1"} 28
modem_signal_dbm{modem="ec25_1"} -57

# Звонки
calls_total{from_modem="ec25_1",to_modem="ec25_2",status="answered"} 5
call_duration_seconds_bucket{from_modem="ec25_1",to_modem="ec25_2",...}

# SMS
sms_total{from_modem="ec25_1",direction="outgoing",status="delivered"} 3
sms_delivery_seconds_bucket{from_modem="ec25_1",to_modem="ec25_2",...}

# Data
data_bytes_rx_total{modem="ec25_1",operator="Tele2"} 1048576
data_bytes_tx_total{modem="ec25_1",operator="Tele2"} 32768
data_sessions_total{modem="ec25_1",operator="Tele2",status="completed"} 2
```

Можно подключить Grafana для визуализации трендов.

## Docker

### Сборка и запуск на RPi

```bash
# Собрать образ
docker buildx build --platform linux/arm64 -t call-tester:latest --load .

# Запуск
docker compose up

# Или отдельные команды
docker compose run --rm call-tester check
docker compose run --rm call-tester run scenarios/full_test.yaml
```

### Деплой с мака

```bash
make docker-build     # собрать образ
make docker-deploy    # передать на RPi
```

В `docker-compose.yml` уже прописан проброс всех USB-устройств и QMI, `privileged: true` для управления сетевыми интерфейсами.

## Справочник AT-команд

### Базовые

| Команда | Описание |
|---------|----------|
| `AT` | Проверка связи (OK) |
| `ATE0` | Выключить эхо |
| `ATI` | Модель/ревизия |
| `AT+CMEE=2` | Расширенные ошибки |
| `AT+CFUN=1` | Полный режим |

### SIM-карта

| Команда | Описание |
|---------|----------|
| `AT+CPIN?` | Статус PIN (READY = ок) |
| `AT+CPIN="1234"` | Ввести PIN |
| `AT+CNUM` | Номер SIM (не все модемы) |
| `AT+CCID` | ICCID SIM |

### Сеть и сигнал

| Команда | Описание |
|---------|----------|
| `AT+CREG?` | Регистрация (0,1 = ок, 0,5 = роуминг) |
| `AT+CSQ` | Сигнал RSSI,BER (0-31, 99=нет) |
| `AT+COPS?` | Оператор |
| `AT+QNWINFO` | Тип сети и частота (Quectel) |

### Голосовые вызовы

| Команда | Описание |
|---------|----------|
| `ATD+7XXX;` | Набор (`;` = голосовой, обязательна!) |
| `ATA` | Ответить на входящий |
| `ATH` / `AT+CHUP` | Положить трубку |
| `AT+CLCC` | Список активных вызовов |
| `AT+CLIP=1` | Определитель номера |
| `ATS0=1` | Автоответ после 1 гудка |
| `AT+VTS="5"` | Отправить DTMF-тон |

### SMS

| Команда | Описание |
|---------|----------|
| `AT+CMGF=1` | Текстовый режим |
| `AT+CMGS="+7..."` | Отправить (после `>` — текст + Ctrl+Z) |
| `AT+CMGR=N` | Прочитать SMS по индексу |
| `AT+CMGD=1,4` | Удалить все SMS |
| `AT+CNMI=2,1,0,0,0` | Уведомления о новых SMS |

### Расшифровка +CLCC

`+CLCC: idx,dir,stat,mode,mpty,"number",type`

| Поле | Значение |
|------|----------|
| `dir` | 0 = исходящий, 1 = входящий |
| `stat` | 0 = active, 2 = dialing, 3 = alerting, 4 = incoming |
| `mode` | 0 = voice |

### Расшифровка уровня сигнала (AT+CSQ)

| RSSI | dBm | Качество |
|------|-----|----------|
| 0-9 | -113...-95 | Плохой |
| 10-14 | -93...-85 | Слабый |
| 15-19 | -83...-75 | Нормальный |
| 20-24 | -73...-65 | Хороший |
| 25-31 | -63...-51 | Отличный |
| 99 | — | Нет сигнала |

## QMI ручная проверка

Если `data_session` не работает — проверь руками:

```bash
# Raw-ip режим
sudo ip link set wwan0 down
echo Y | sudo tee /sys/class/net/wwan0/qmi/raw_ip
sudo ip link set wwan0 up

# Старт QMI-сессии
sudo qmicli -d /dev/cdc-wdm0 --device-open-proxy \
  --wds-start-network="apn=internet,ip-type=4" \
  --client-no-release-cid

# DHCP
sudo udhcpc -i wwan0

# Проверка
ping -I wwan0 -c 3 8.8.8.8
curl --interface wwan0 https://ifconfig.me

# Счётчики трафика
cat /sys/class/net/wwan0/statistics/rx_bytes
cat /sys/class/net/wwan0/statistics/tx_bytes
```

## Частые проблемы

**Permission denied на /dev/ttyUSB***
→ `sudo usermod -aG dialout $USER` + перелогиниться

**Модем не отвечает на AT**
→ ModemManager захватил порт: `sudo systemctl stop ModemManager && sudo systemctl disable ModemManager`

**QMI не стартует**
→ Интерфейс не в raw-ip: `echo Y | sudo tee /sys/class/net/wwan0/qmi/raw_ip`
→ Или модем не в QMI-режиме: `AT+QCFG="usbnet",0` через picocom, потом перезагрузить модем

**Нет интернета через wwan0**
→ Policy routing: `ip rule show`, `ip route show table 100`
→ Проверить вручную: `curl --interface wwan0 https://ifconfig.me`

**Звонок не устанавливается**
→ Голосовой сервис не активен на SIM, или нет покрытия
→ Проверить сигнал: `AT+CSQ` (значение 10+ нормально)
→ Проверить регистрацию: `AT+CREG?` (должно быть 0,1 или 0,5)

**SMS не доставляется**
→ Баланс SIM (проверить через `AT+CUSD=1,"*100#",15` если оператор поддерживает)
→ Переполнена память SMS: `AT+CMGD=1,4` удалит все

**InvalidHandle при отключении QMI**
→ Сессия уже закрылась — не критично, можно игнорировать

## Масштабирование до 30 модемов

- **USB-хабы:** активный USB 3.0 хаб с внешним питанием (каждый EC25 потребляет до 500мА)
- **udev-правила:** обязательно, иначе порядок портов меняется при перезагрузке (различать по серийнику)
- **Горутины:** 30 goroutines + WaitGroup = ничтожная нагрузка для Go runtime
- **CPU:** <5% на RPi 5 при 30 параллельных звонках
- **RAM:** ~30 МБ на весь процесс
- **Bottleneck:** скорость AT-команд (~10-50мс) и сотовая сеть, не язык

## Лицензия

MIT
