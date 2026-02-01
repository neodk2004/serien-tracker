---
description: Finalisierung und Start des Projekts
---

Dieser Workflow automatisiert den Prozess der Finalisierung und startet die Anwendung in einer Docker-Umgebung.

1. Erstelle eine `.env` Datei basierend auf der `.env.example` (falls nicht vorhanden).
   ```powershell
   if (!(Test-Path .env)) { Copy-Item .env.example .env }
   ```

2. Starte die Docker-Container.
// turbo
   ```powershell
   docker compose up --build -d
   ```

3. Überprüfe den Status der Container.
   ```powershell
   docker compose ps
   ```

4. Zeige die Logs der Web-Anwendung an.
   ```powershell
   docker compose logs -f web
   ```
