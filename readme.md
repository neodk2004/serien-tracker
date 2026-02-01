# 🎬 Serien-Tracker (Go Web Application)

Ein moderner, datengetriebener Serientracker mit einem **"Nerd Dashboard"**. Die Anwendung nutzt die OMDb API für Informationen, PostgreSQL für die Persistenz und Docker für ein schmerzfreies Deployment.

![Nerd Dashboard Preview](Screenshot%202025-11-22%20124254.png)

## 🚀 Features

*   **Nerd-Statistiken**: Globaler Rang, Binge-Zeit-Berechnung (mit Vergleichen wie "Herr der Ringe" Durchläufen) und Fortschrittsbalken.
*   **Globales Leaderboard**: Vergleiche deinen Fortschritt mit anderen Nutzern.
*   **Admin Panel**: Benutzerverwaltung (Passwort-Reset, Löschen, Hinzufügen) direkt in der Web-Oberfläche.
*   **Responsive Design**: Optimiert für Desktop, Tablet und Smartphone (Netflix-Aesthetic).
*   **Avatar-System**: Automatische Initialen-Avatare basierend auf dem Benutzernamen.
*   **PDF-Export**: Exportiere deine gesamte Liste als professionelles Dokument.

---

## 🛠️ Schnellstart mit Docker 🐳

Die Anwendung ist vollständig containerisiert. Du musst Go nicht lokal installiert haben.

### 1. Voraussetzungen
- **Docker & Docker Compose** installiert.
- Einen **OMDb API-Key** (kostenlos unter [omdbapi.com](https://www.omdbapi.com/apikey.aspx)).

### 2. Setup
```bash
# Repository klonen
git clone https://github.com/dein-benutzername/serientracker.git
cd serientracker

# Konfigurationsdatei erstellen
cp .env.example .env
```

### 3. Konfiguration
Öffne die `.env` Datei und trage deinen Key ein:
```env
OMDB_API_KEY=dein_key_hier
SESSION_SECRET=ein_sehr_langer_geheimer_string
```

### 4. Starten
```bash
docker compose up --build -d
```

---

## 🖥️ Zugriff
Sobald die Container laufen:
- **Web-App**: [http://localhost:8081](http://localhost:8081)
- **Admin-Bereich**: Registriere den ersten Nutzer – dieser wird automatisch Administrator.
- **E-Mail Test (MailHog)**: [http://localhost:8025](http://localhost:8025)

---

## 🏗️ Projektstruktur
```text
serientracker/
├── main.go               # Backend (Go / Fiber/Standard Library)
├── templates/            # HTML-Templates (Go Templates)
├── static/               # CSS & JS (Vanilla UI)
├── fonts/                # PDF-Fonts (DejaVu)
├── docker-compose.yml    # Infrastruktur (PostgreSQL, App, MailHog)
└── .env.example          # Template für Einstellungen
```

## 🔒 Sicherheit
- **Passwort-Hashing**: Gesichert mit bcrypt.
- **Datenbank**: PostgreSQL 16 (Alpine).
- **Git-Protection**: Sensible Daten (.env, .db) sind über `.gitignore` geschützt.

---
*Hinweis: Dies ist ein persönliches Projekt und nicht mit IMDb oder OMDb affiliiert.*
