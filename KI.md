# KI.md - Gemini CLI Project Analysis

## Project: series_tracker (Pre-Alpha Version)

### Overview
This project, "series_tracker," is a series tracking application implemented in Go. It aims to help users manage their watched series, fetch details from the OMDb API, and visualize their progress.

### Project Structure & Components:

**Go Web Application (Primary Interface)**
*   **Entry Point:** `main.go`
*   **Framework/Libraries:**
    *   `net/http`: For serving web content.
    *   `html/template`: For rendering dynamic HTML pages.
    *   `github.com/jung-kurt/gofpdf`: For generating PDF exports of series lists.
    *   OMDb API Integration: Fetches series details (title, year, IMDb ID, poster, total seasons/episodes) and search results.
*   **Data Storage:** Initially `series.json` (JSON file), then migrated to SQLite, and now **PostgreSQL**.
*   **Features:**
    *   Web-based user interface with a Netflix-inspired design (`static/css/style.css`).
    *   User authentication (registration, login, logout, password reset).
    *   Admin workflow (first registered user is admin).
    *   Add series by title or IMDb ID.
    *   Update watched episodes.
    *   Delete series.
    *   Display series in a list or poster view (`templates/index.html`, `templates/mylist.html`).
    *   Search for series using the OMDb API.
    *   Generate and download PDF of the series list.
    *   Display basic statistics (`templates/stats.html`).
    *   Automatic fetching of missing cover URLs.
    *   Automatic port finding for the web server.
*   **API Key:** Hardcoded within `main.go` (`fbd55d5e`).

**Static Assets & Templates:**
*   `static/css/style.css`: Provides Netflix-like styling for the web application.
*   `templates/*.html`: Go HTML templates (`index.html`, `mylist.html`, `stats.html`, `login.html`, `register.html`, `forgot_password.html`, `reset_password.html`) for the web UI.
*   `fonts/`: Contains custom fonts (`DejaVuSans-back.ttf`, `DejaVuSans.ttf`).

**Configuration Files:**
*   `go.mod`: Go module definition and dependency management.
*   `Dockerfile`: Defines how to build the Go application into a Docker image.
*   `docker-compose.yml`: Defines services (web app, PostgreSQL database, MailHog) for local development and testing.

### Key Observations & Feedback:

1.  **API Key Handling:** The OMDb API key is hardcoded within `main.go`.
    *   **Recommendation:** For security and flexibility, this should be loaded from environment variables or a configuration file.

2.  **Configuration Management:** Database and session secret keys are now externalized to environment variables, which is a significant improvement for containerized deployment.

3.  **Documentation:** The `readme.md` should be updated to accurately reflect the current state of the project, including the Docker Compose setup and PostgreSQL database.

---

## Update: Implementation of User Authentication and Database Migration (31. Januar 2026)

The Go web application (`main.go`) was refactored to introduce a robust user authentication system and migrated from file-based JSON data storage to an SQLite database.

### Summary of Changes:

1.  **Database Integration (SQLite):** `github.com/mattn/go-sqlite3` was integrated.
2.  **Data Migration:** A `migrateSeriesJSONtoDB()` function was implemented to automatically migrate existing series data from `series.json` to SQLite.
3.  **User Authentication & Session Management:** Implemented using `golang.org/x/crypto/bcrypt` for password hashing and `github.com/gorilla/sessions` for secure cookie-based session management.
4.  **New Database Interaction Functions:** Helper functions for database CRUD operations (`getUsers`, `getUserByName`, `addSeriesToDB`, etc.) were introduced.
5.  **Handler Updates:** All existing HTTP handlers were updated to integrate with the new authentication system and database functions.
6.  **Authentication Handlers & Templates:** New handlers (`loginHandler`, `registerHandler`, `logoutHandler`) and HTML templates (`login.html`, `register.html`) were created.

---

## Update: Implementation of "Forgot Password" Flow (31. Januar 2026)

The password reset functionality was fully implemented for the Go web application.

### Summary of Changes:

1.  **Database Schema Update:** `users` table was augmented with `reset_token` and `reset_token_expires_at` columns.
2.  **Token Generation:** `generateResetToken()` using `crypto/rand` and `encoding/base64`.
3.  **Email Sending:** `sendResetEmail()` using Go's `net/smtp` and `net/mail`. Configured to use `localhost:1025` for development (suitable for MailHog).
4.  **New Handlers:** `forgotPasswordHandler` and `resetPasswordHandler`.
5.  **New HTML Templates:** `forgot_password.html` and `reset_password.html`.

---

## Update: Implementation of "First User as Admin" Workflow (31. Januar 2026)

The Go web application (`main.go`) was updated to implement a workflow where the very first user to register automatically becomes an administrator.

### Summary of Changes:

1.  **Database Schema Update:** `users` table was augmented with an `is_admin` column.
2.  **`registerHandler` Modification:** Logic added to set `is_admin` to `true` for the first registered user.

---

## Update: Module Loading Guidance (31. Januar 2026)

In-code guidance was added to `main.go` for resolving Go module dependency issues (`go mod tidy`, `go mod download`).

---

## Error Resolution: CGO_ENABLED=0 & C Compiler (gcc) not found (31. Januar 2026)

Initially, the application faced issues due to `go-sqlite3` requiring CGO and a C compiler (`gcc`).

**Problem:** `go-sqlite3` requires CGO to function, and CGO requires a C compiler (like `gcc`) to build. This was causing compilation errors on Windows development environments where `gcc` was not installed or not in the system's PATH.

**Resolution during development:**
*   Ensured `CGO_ENABLED=1` was set for `go run` or `go build`.
*   Provided instructions for installing MinGW-w64 (which includes `gcc`) and adding it to the system PATH on Windows.

**Note:** This developer-level setup was necessary for building the application while it used `go-sqlite3`. However, to provide a simpler experience for end-users and a more robust Dockerized deployment, the database was subsequently refactored to PostgreSQL (see next section).

---

## Refactoring: Switch from SQLite to PostgreSQL (31. Januar 2026)

To address the complexities of `go-sqlite3` requiring CGO and a C compiler during the Go application build, and to establish a more robust, decoupled, and standard database setup for containerized deployment, the application's database backend was refactored from SQLite to PostgreSQL.

### Rationale:
*   **Eliminate CGO dependency:** Switching to PostgreSQL removes the need for `gcc` during the Go application's build process, simplifying the developer environment setup.
*   **Robust Containerization:** PostgreSQL is a client-server database, ideal for multi-service Docker Compose setups, offering better scalability and manageability than file-based SQLite in such an environment.
*   **Production Readiness:** Adopts a more industry-standard database solution for web applications.

### Summary of Changes:

1.  **`go.mod` Updates:**
    *   Removed `github.com/mattn/go-sqlite3` and `modernc.org/sqlite`.
    *   Added `github.com/lib/pq` (PostgreSQL driver).

2.  **`main.go` Changes:**
    *   **Imports:** Changed `_ "github.com/mattn/go-sqlite3"` to `_ "github.com/lib/pq"`.
    *   **PostgreSQL Constants:** Added `defaultDBHost`, `defaultDBPort`, `defaultDBUser`, `defaultDBPassword`, `defaultDBName`, `sslMode` for PostgreSQL connection configuration, with values that can be overridden by environment variables.
    *   **`initDB()` Function:**
        *   Modified to construct a PostgreSQL connection string from environment variables (with fallbacks to defaults).
        *   Changed `sql.Open("sqlite3", dbPath)` to `sql.Open("postgres", connStr)`.
        *   Adapted `CREATE TABLE users` DDL: `INTEGER PRIMARY KEY AUTOINCREMENT` to `SERIAL PRIMARY KEY`, `DATETIME` to `TIMESTAMPTZ`, `INTEGER DEFAULT 0` to `BOOLEAN DEFAULT FALSE`.
        *   Adapted `CREATE TABLE series` DDL: `INTEGER PRIMARY KEY AUTOINCREMENT` to `SERIAL PRIMARY KEY`, added `VARCHAR` length constraints, and changed `imdb_id TEXT NOT NULL UNIQUE` to `UNIQUE (user_id, imdb_id)` for per-user uniqueness.
    *   **`migrateSeriesJSONtoDB()` Function:** Updated `INSERT` statements to use PostgreSQL's dollar-sign placeholders (`$1, $2, ...`) instead of question marks (`?`).
    *   **CRUD Functions (Placeholder Syntax):** All SQL queries within `getUserByName`, `getUserByID`, `getAllSeriesForUser`, `addSeriesToDB`, `updateSeriesInDB`, `deleteSeriesFromDB`, `updateMissingCovers`, `registerHandler`, `forgotPasswordHandler`, and `resetPasswordHandler` were updated to use PostgreSQL's `$1, $2, ...` placeholders.
    *   **`LastInsertId()` Replacements:** For `addSeriesToDB` and `registerHandler`, `db.Exec` followed by `res.LastInsertId()` was replaced with `db.QueryRow` using the `RETURNING id` clause, which is the idiomatic way to get a newly inserted ID in PostgreSQL.

3.  **`Dockerfile` Updates:**
    *   **Builder Stage:** Removed `gcc` and `musl-dev` from `apk add --no-cache` command. `git` remains as it's needed for `go mod download`.
    *   **Runner Stage:** Removed `sqlite-libs` as it's no longer necessary.

4.  **`docker-compose.yml` Updates:**
    *   **New `db` Service:** Added a PostgreSQL service (`image: postgres:16-alpine`) with environment variables (`POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`), a persistent volume (`pg_data`), and a healthcheck.
    *   **`web` Service Configuration:**
        *   Updated `environment` variables to include `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` for PostgreSQL connection.
        *   Updated `depends_on` to include `db`.
        *   Removed `series_data` volume (which was for SQLite).
    *   **`volumes` Section:** Replaced `series_data` with `pg_data`.

### Instructions to Run with Docker Compose:

1.  **Ensure Docker is Running:** Make sure Docker Desktop (or your Docker environment) is running on your machine.
2.  **Open Terminal:** Navigate to the project's root directory in your terminal.
3.  **Build and Run:** Execute the following command:
    ```bash
    docker compose up --build
    ```
    *   `--build` ensures that your Go application's Docker image is rebuilt with the latest changes.
    *   This will start the `db`, `web`, and `mailhog` services. Docker Compose will automatically create the `series_tracker_network` and `pg_data` volume.
4.  **Access the Web Application:** Once the services are up, open your web browser and go to `http://localhost:8081`.
5.  **Access MailHog Web UI:** To test password reset emails, open `http://localhost:8025` in your browser. Any emails sent by the application (e.g., password reset requests) will appear here.
6.  **Stop Services:** To stop and remove the containers, networks, and volumes (but preserve named volumes like `pg_data` by default), run:
    ```bash
    docker compose down
    ```
    To stop and remove everything, including named volumes:
    ```bash
    docker compose down -v
    ```
    (Use with caution, as this will delete your PostgreSQL data!)

---

### Rust CLI Application (Separate Note):

The Rust CLI application (`src/main.rs`) was noted in the initial project analysis. This refactoring focuses solely on the Go web application. If the Rust CLI is intended to be used with the new PostgreSQL database, its `src/main.rs` would also need significant modifications to connect to PostgreSQL using a Rust PostgreSQL client library (e.g., `postgres` crate) and adapt its SQL queries. It is not currently included in the Docker Compose setup as it is a CLI tool rather than a long-running service.