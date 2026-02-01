# Serien-Tracker (Go Web Application)

Ein robuster und effizienter Serientracker als Webanwendung, geschrieben in Go. Die Anwendung nutzt die OMDb API, um Serieninformationen abzurufen, ermöglicht Benutzern die Verwaltung persönlicher Serienlisten und bietet Funktionen zur Authentifizierung und Datenpersistenz mit PostgreSQL.

## Funktionen

*   **Benutzerauthentifizierung:** Registrierung, Login, Logout mit sicherer Passwort-Hash-Funktion (bcrypt) und Session-Management.
*   **Passwort zurücksetzen:** Sichere Möglichkeit, Passwörter per E-Mail zurückzusetzen.
*   **Admin-Funktionalität:** Der erste registrierte Benutzer wird automatisch zum Administrator.
*   **Serienverwaltung:** Hinzufügen, Aktualisieren und Löschen von Serien über Titel oder IMDb-ID.
*   **Folgenstatus verwalten:** Verfolge den Fortschritt deiner Serien.
*   **OMDb API-Integration:** Abrufen von vollständigen Serieninformationen (Titel, Staffeln, Episoden, Poster, etc.).
*   **PDF-Export:** Exportiere deine Serienliste als PDF.
*   **Statistiken:** Übersicht über deine Serien.
*   **Persistente Daten:** Speicherung aller Benutzer- und Seriendaten in einer PostgreSQL-Datenbank.
*   **Docker-Containerisierung:** Einfaches Setup und Ausführung der gesamten Anwendung (Web-App, Datenbank, Mail-Server) mit Docker Compose.

## Voraussetzungen

*   **Go:** Version 1.22 oder höher. (Nicht direkt für die Docker-Ausführung, aber für Entwicklung wichtig)
*   **Docker & Docker Compose:** Erforderlich für die lokale Entwicklung und den Betrieb der Anwendung (inkl. PostgreSQL und MailHog).
*   **OMDb API-Schlüssel:** Registriere dich kostenlos unter [https://www.omdbapi.com/apikey.aspx](https://www.omdbapi.com/apikey.aspx)
*   **Umgebungsvariablen:** Die Anwendung wird primär über Umgebungsvariablen konfiguriert (siehe `.env.example`).

## Schnellstart mit Docker 🐳

Die einfachste Methode, den Serien-Tracker zu nutzen, ist über Docker Compose. Dies startet die Go-Anwendung, eine PostgreSQL-Datenbank und MailHog (zum Testen von E-Mails).

### 1. Voraussetzungen
Stelle sicher, dass **Docker** und **Docker Compose** auf deinem System installiert sind.

### 2. Installation & Konfiguration
Klone das Repository und erstelle deine Konfigurationsdatei:

```bash
# Repository klonen
git clone https://github.com/dein-benutzername/serientracker.git
cd serientracker

# Umgebungsvariablen konfigurieren
cp .env.example .env
```

Öffne die `.env` Datei und trage deinen **OMDb API-Key** ein. Du kannst dort auch Passwörter und Ports anpassen.

### 3. Anwendung starten
Starte alle Dienste mit einem einzigen Befehl:

```bash
docker compose up --build -d
```

### 4. Zugriff
Sobald die Container laufen, erreichst du die Anwendung unter:
- **Web-Oberfläche:** [http://localhost:8081](http://localhost:8081)
- **MailHog (E-Mail Test):** [http://localhost:8025](http://localhost:8025)

---

## Manuelle Installation & Ausführung (Skript)

Falls du ein automatisiertes Skript bevorzugst, kannst du dieses nutzen:

## Wichtige Hinweise für die Entwicklung

*   **Session Secret:** Das `SESSION_SECRET` in `docker-compose.yml` ist als `"your-super-secret-key"` gesetzt. **Ändere dies in der Produktion zu einem langen, zufälligen und sicheren Wert!**
*   **PostgreSQL-Anmeldedaten:** Die Datenbank-Anmeldedaten (`DB_USER`, `DB_PASSWORD`, `DB_NAME`) sind in `docker-compose.yml` definiert und sollten für die Produktion ebenfalls geändert werden.
*   **Datenpersistenz:** Die PostgreSQL-Daten werden im Docker-Volume `pg_data` gespeichert. Diese Daten bleiben erhalten, auch wenn die Container gestoppt oder neu erstellt werden.

## Dienste stoppen

Um die Docker-Dienste anzuhalten und zu entfernen (benannte Volumes wie `pg_data` bleiben standardmäßig erhalten):
```bash
docker compose down
```

Um alle Daten (einschließlich `pg_data`-Volume) zu entfernen (VORSICHT: Dies löscht deine Datenbankdaten dauerhaft!):
```bash
docker compose down -v
```

## Projektstruktur

```
serientracker/
├── main.go               # Hauptprogramm (Go Web App)
├── go.mod                # Go Moduldefinition
├── go.sum                # Go Modul-Prüfsummen
├── Dockerfile            # Dockerfile für die Go Web App
├── docker-compose.yml    # Docker Compose Konfiguration
├── templates/            # HTML-Templates
│   ├── index.html
│   ├── login.html
│   ├── register.html
│   ├── forgot_password.html
│   ├── reset_password.html
│   ├── mylist.html
│   └── stats.html
├── static/               # Statische Dateien (CSS, JS, Bilder)
│   └── css/
│       └── style.css
├── fonts/                # Schriftarten
│   ├── DejaVuSans-back.ttf
│   └── DejaVuSans.ttf
├── KI.md                 # Projektdokumentation und Entwicklungsnotizen
└── README.md             # Dieses Handbuch
```

## Beiträge

Beiträge sind willkommen! Bitte erstellt ein Issue oder Pull Request für Verbesserungen.

Hinweis: Dieser Serientracker ist ein persönliches Projekt und nicht mit IMDb oder OMDb affiliiert.
