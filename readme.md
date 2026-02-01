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

## Installation & Ausführung (Endbenutzerfreundliches Skript)

Dieses "Installationsskript" führt die notwendigen Schritte aus, um die Anwendung über Docker Compose auf Ihrem System einzurichten und zu starten. Es wird überprüft, ob Docker und Docker Compose installiert sind, das Repository geklont und die Anwendung gestartet.

**Wichtig:** Der OMDb API-Schlüssel kann über die Umgebungsvariable `OMDB_API_KEY` oder in einer `.env` Datei gesetzt werden. Falls keine Variable gesetzt ist, wird ein Standard-Schlüssel verwendet, der jedoch limitiert sein kann.

```bash
#!/bin/bash

# --- 1. Überprüfen der Voraussetzungen ---
 echo "--- Überprüfe die Installation von Docker und Docker Compose ---"
if ! command -v docker &> /dev/null
then
    echo "Docker ist nicht installiert. Bitte installieren Sie Docker Desktop (https://www.docker.com/products/docker-desktop) oder Ihren Docker-Client."
    exit 1
fi

# Überprüfen, ob `docker compose` oder `docker-compose` verfügbar ist
if command -v docker compose &> /dev/null
then
    DOCKER_COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null
then
    DOCKER_COMPOSE_CMD="docker-compose"
else
    echo "Docker Compose ist nicht installiert. Bitte installieren Sie es."
    echo "Wenn Sie Docker Desktop verwenden, ist es bereits enthalten."
    exit 1
fi
 echo "Docker und Docker Compose ($DOCKER_COMPOSE_CMD) sind installiert."
 echo ""

# --- 2. Repository klonen (falls noch nicht geschehen) ---
REPO_URL="https://github.com/dein-benutzername/serientracker.git" # ERSETZE DIES DURCH DEIN AKTUELLES REPO!
REPO_DIR="serientracker"

if [ ! -d "$REPO_DIR" ]; then
    echo "--- Klone das Repository von $REPO_URL ---"
    git clone "$REPO_URL" "$REPO_DIR"
    cd "$REPO_DIR"
else
    echo "--- Repository '$REPO_DIR' existiert bereits. Wechsle in das Verzeichnis. ---"
    cd "$REPO_DIR"
    echo "--- Ziehe die neuesten Änderungen ---"
    git pull
fi

 echo ""
 echo "--- HINWEIS ZUM OMDb API-SCHLÜSSEL ---"
 echo "Der OMDb API-Schlüssel sollte idealerweise über eine .env Datei oder"
 echo "Umgebungsvariable gesetzt werden. Siehe .env.example für Details."
 echo "Drücken Sie ENTER, um fortzufahren oder STRG+C, um abzubrechen..."
 read -r

# --- 4. Docker Compose starten ---
 echo "--- Starte die Anwendung mit Docker Compose ---"
 echo "Dies wird das Go-Anwendungs-Image erstellen, die PostgreSQL-Datenbank und MailHog starten."
"$DOCKER_COMPOSE_CMD" up --build -d # -d für detached mode (im Hintergrund)

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ Anwendung erfolgreich gestartet!"
 echo "🚀 Die Serientracker Web-Oberfläche ist erreichbar unter: http://localhost:8081"
 echo "📧 Die MailHog Web-Oberfläche (zum Testen von E-Mails) ist erreichbar unter: http://localhost:8025"
 echo ""
 echo "Um die Logs zu sehen, nutzen Sie: $DOCKER_COMPOSE_CMD logs -f"
 echo "Um die Dienste zu stoppen: $DOCKER_COMPOSE_CMD down"
else
    echo "❌ Fehler beim Starten der Anwendung mit Docker Compose."
    echo "Bitte überprüfen Sie die Fehlermeldungen oben."
fi
```

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
