# qBitCtrl Go

Panel zarządzania qBittorrent — przebudowany w Go.

## Porównanie z wersją Python

| | Python/Flask | Go |
|---|---|---|
| RAM | ~80–150 MB | ~8–15 MB |
| Start | ~2s | <50ms |
| Rozmiar | 90KB .py + deps | 5.7MB single binary |
| Zależności | flask, requests, arrapi | **zero** |
| CPU idle | ~2–5% | ~0.1% |

## Szybki start

```bash
# Pobierz odpowiedni plik dla swojej architektury
./qbitctrl-linux-amd64   # standardowy serwer x86_64
./qbitctrl-linux-arm64   # Raspberry Pi 4, nowoczesny ARM
./qbitctrl-linux-armv7   # Raspberry Pi 3/2, starszy ARM

# Panel dostępny na
http://localhost:9911
```

## Konfiguracja (zmienne środowiskowe)

```bash
QBITCTRL_PORT=9911              # port HTTP (domyślnie 9911)
QBITCTRL_DB=servers.json        # plik z serwerami
QBITCTRL_ARR=arr.json           # plik z konfig Radarr/Sonarr
```

## Instalacja jako systemd service

```bash
cp qbitctrl-linux-amd64 /usr/local/bin/qbitctrl
chmod +x /usr/local/bin/qbitctrl
cp qbitctrl.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now qbitctrl

# Logi
journalctl -u qbitctrl -f
```

## Build ze źródeł

```bash
# Wymagania: Go 1.22+
git clone ...
cd qbitctrl-go
make build        # build lokalny
make dist         # build wszystkich platform
make install      # instalacja systemd
```

## Migracja z wersji Python

Pliki JSON są kompatybilne — skopiuj `qbitctrl_servers.json` i `qbitctrl_arr.json`
do katalogu roboczego Go i uruchom. Wszystkie serwery i konfiguracja ARR zostaną
automatycznie załadowane.
