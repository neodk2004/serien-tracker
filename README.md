# Serientracker (Go)
Ein einfacher und effizienter Serientracker, geschrieben in Go, der die OMDb API nutzt, um Serieninformationen abzurufen und persönliche Serienlisten zu verwalten.

# Funktionen
Serien hinzufügen über Titel oder IMDb-ID

Folgenstatus verwalten (Anzahl der gesehenen Folgen)

Vollständige Serieninformationen (Titel, Staffeln, Episoden, Bewertung, etc.)

PDF-Export der Serienliste zum Teilen mit Freunden

Lokale Datenspeicherung im JSON-Format

# Voraussetzungen
Go 1.16 oder höher

OMDb API-Schlüssel (kostenlos registrierbar unter https://www.omdbapi.com/apikey.aspx)


Installation
Repository klonen:
`git clone https://github.com/dein-benutzername/serientracker.git`
`cd serientracker`

Abhängigkeiten installieren:

`go mod download`

OMDb API-Schlüssel konfigurieren:

`export OMDB_API_KEY="dein_api_schluessel"`

Verwendung

Serien hinzufügen

`go run main.go add --titel "Breaking Bad"`

oder mit IMDb-ID

`go run main.go add --id "tt0903747"`

Folgenstatus aktualisieren

`go run main.go update --id "tt0903747" --episoden 5`
Serienliste anzeigen

`go run main.go list`

PDF exportieren

`go run main.go export --output meine_serien.pdf`

# Projektstruktur

	serientracker/
	├── main.go          # Hauptprogramm
	├── fonts/           # Fonts und Schriftarten
	├── static/          # Style-Sheet
	    └── css/
	        └── style.css
	├── templates/         # HTML - Pfad
	    └── index.html
	    └── mylist.html
	└── README.md

# Konfiguration
Die Anwendung verwendet folgende Umgebungsvariablen:

OMDB_API_KEY - OMDb API Schlüssel (erforderlich)

Den Key musst du in der Zeile 71 in der main.go eingeben. 
Falls das nicht passiert, erhälst du eine Fehlermeldung: "WARNUNG: Bitte trage deinen echten OMDb API-Key in die main.go ein"



# Beiträge sind willkommen! Bitte erstellt ein Issue oder Pull Request für Verbesserungen.

# Hinweis: Dieser Serientracker ist ein persönliches Projekt und nicht mit IMDb oder OMDb affiliiert.



