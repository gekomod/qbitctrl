# 🚀 qBitCtrl - Webowy menedżer qBittorrent

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**qBitCtrl** to lekki, nowoczesny webowy interfejs do zarządzania wieloma instancjami qBittorrent. Obsługuje zdalny restart przez SSH/Docker/systemd, monitorowanie prędkości, integrację z Radarr/Sonarr i wiele więcej.

---

## ✨ Funkcje

| Funkcja | Opis |
|---------|------|
| 📊 **Dashboard** | Podgląd wszystkich serwerów qBittorrent w jednym miejscu |
| 🧲 **Zarządzanie torrentami** | Dodawanie, pauza, wznawianie, usuwanie (również z plikami) |
| 📈 **Wykresy prędkości** | Historia prędkości z ostatnich 60 sekund |
| 🔄 **Auto-restart** | Automatyczne restartowanie offline'owych serwerów przez SSH |
| 🎬 **ARR Integration** | Wyszukiwanie i dodawanie filmów/seriali z Radarr/Sonarr |
| 🔔 **Powiadomienia** | Powiadomienia browser o ukończonych torrentach |
| 🌙 **Motyw** | Jasny / ciemny - zapamiętywany w localStorage |
| 📦 **Eksport CSV** | Eksport listy torrentów do pliku CSV |
| 🖥️ **Multi-server** | Obsługa wielu serwerów qBittorrent jednocześnie |

---

## 🚀 Szybki start

### Docker (zalecany)

```bash
docker run -d \
  --name qbitctrl \
  -p 9911:9911 \
  -v qbitctrl-data:/data \
  ghcr.io/gekomod/qbitctrl:latest
```

# Pobierz najnowszą wersję
```bash
wget https://github.com/gekomod/qbitctrl/releases/latest/download/qbitctrl-linux-amd64
```

# Uruchom
```bash
chmod +x qbitctrl-linux-amd64
./qbitctrl-linux-amd64
Z źródła
bash
git clone https://github.com/gekomod/qbitctrl.git
cd qbitctrl
make build
./qbitctrl
```

## 🔧 Konfiguracja
Otwórz przeglądarkę na http://localhost:9911

Zaloguj się:

Login: admin

Hasło: adminadmin

Dodaj serwer qBittorrent (Host, Port, login, hasło)

Gotowe! 🎉

Auto-restart (opcjonalny)
Aby działało automatyczne restartowanie offline'owych serwerów:

Wygeneruj klucz SSH: ssh-keygen -t ed25519 -f ~/.ssh/qbitctrl

Skopiuj na serwer: ssh-copy-id -i ~/.ssh/qbitctrl.pub root@ip_serwera

W konfiguracji serwera w qBitCtrl:

Typ restartu: docker lub systemd

Nazwa unita/kontenera

SSH User, Port, ścieżka do klucza

## 📦 Wymagania
qBittorrent v4.3+ (WebUI włączony)

Go 1.22+ (tylko do kompilacji)

Docker (opcjonalnie)

SSH dostęp (opcjonalnie, do auto-restartu)

## 🛠️ Development

bash

# Klonowanie
```bash
git clone https://github.com/gekomod/qbitctrl.git
cd qbitctrl
```

# Instalacja zależności
```bash
go mod download
```

# Uruchomienie (hot-reload)
```bash
go run ./cmd/server
```

# Build
```bash
make build
```


## 🐛 Znane problemy
Przy bardzo dużej liczbie torrentów (>2000) może spowalniać odświeżanie

WebUI qBittorrent musi być dostępny z poziomu qBitCtrl (brak CORS problemów)

## 🤝 Contributing
Fork repozytorium

Stwórz branch: git checkout -b feature/amazing-feature

Commit: git commit -m 'Add amazing feature'

Push: git push origin feature/amazing-feature

Open Pull Request

## 📝 Licencja
MIT License - zobacz plik LICENSE

## 🙏 Podziękowania
qBittorrent - za świetne WebAPI

Radarr i Sonarr - za integrację

Wszystkim contributorom!

## 📞 Kontakt
GitHub Issues - https://github.com/gekomod/qbitctrl/issues

Discord - (opcjonalnie)
