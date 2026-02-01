<<<<<<< HEAD
package main

import (
	"bytes"
	"database/sql"
	"encoding/json" // Temporarily added for migration
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os" // Permanently added for environment variable access
	"sort"
	"strconv"
	"strings"
	"time"

	"context"         // New import for context
	"crypto/rand"     // New import for secure token generation
	"encoding/base64" // New import for encoding tokens
	"net/mail"        // New import for email address parsing
	"net/smtp"        // New import for sending emails

	"github.com/gorilla/sessions" // New import for sessions
	"github.com/jung-kurt/gofpdf"
	_ "github.com/lib/pq" // PostgreSQL driver
	"golang.org/x/crypto/bcrypt"
)

type Series struct {
	ID              int
	Title           string
	Year            string
	IMDBID          string
	EpisodesWatched int
	TotalEpisodes   int
	Status          string
	Progress        int
	CoverURL        string `db:"cover_url"`
	UserID          int    // Foreign key to users table
}

// OldSeries struct for JSON migration
type OldSeries struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	Year            string `json:"year"`
	IMDBID          string `json:"imdb_id"`
	EpisodesWatched int    `json:"episodes_watched"`
	TotalEpisodes   int    `json:"total_episodes"`
	Status          string `json:"status"`
	Progress        int    `json:"progress"`
	CoverURL        string `json:"CoverURL"`
	User            string `json:"user"`
}

type User struct {
	ID           int
	Username     string
	PasswordHash string
	Email        string
	CreatedAt    time.Time
	IsAdmin      bool   // New field for admin status
	AvatarURL    string // Persistent avatar selection
}

type OMDbResponse struct {
	Title        string `json:"Title"`
	Year         string `json:"Year"`
	TotalSeasons string `json:"totalSeasons"`
	IMDBID       string `json:"imdbID"`
	Response     string `json:"Response"`
	Error        string `json:"Error"`
	Poster       string `json:"Poster"`
}

type SearchResult struct {
	Search       []SearchItem `json:"Search"`
	Response     string       `json:"Response"`
	Error        string       `json:"Error"`
	TotalResults string       `json:"totalResults"`
}

type SearchItem struct {
	Title  string `json:"Title"`
	Year   string `json:"Year"`
	IMDBID string `json:"imdbID"`
	Type   string `json:"Type"`
	Poster string `json:"Poster"`
}

type UserWithAvatar struct {
	ID       int
	Username string
	Initial  string
}

type UserStatsData struct {
	User            string
	EpisodesWatched int
	EpisodesTotal   int
	Progress        int
	Completed       int
	WatchTimeHours  int
	Rank            int
}

type PageData struct {
	SeriesList     []Series
	SearchResults  []SearchItem
	SearchQuery    string
	ErrorMessage   string
	SuccessMessage string
	APIAvailable   bool
	TotalSeries    int
	TotalWatched   int
	TotalEpisodes  int
	WatchTimeHours int
	Rank           int

	SortBy    string
	Order     string
	UserStats []UserStatsData

	User          *User            // Current logged-in user object
	CurrentUser   UserWithAvatar   // Current user info
	Users         []UserWithAvatar // All users for switcher
	FullUsers     []User           // Detailed list for admin
	CurrentUserID int
}

// Define a type for context keys to avoid collisions
type contextKey string

const (
	dbPath   = "data/series.db" // Updated path for Docker volume persistence
	dataFile = "series.json"    // Needed for migration

	sessionName               = "series-tracker-session"
	userSessionKey            = "userID"
	userContextKey contextKey = "user" // Key to store User in request context

	// PostgreSQL specific constants (default values - will be overwritten by environment variables)
	defaultDBHost     = "localhost"
	defaultDBPort     = "5432"
	defaultDBUser     = "user"
	defaultDBPassword = "password"
	defaultDBName     = "seriestracker"
	defaultSSLMode    = "disable" // For local/Docker testing; use "require" or "verify-full" in production
)

var (
	apiKey = os.Getenv("OMDB_API_KEY") // Load API Key from environment variable
)

var (
	templates    *template.Template
	db           *sql.DB               // Global database connection
	sessionStore *sessions.CookieStore // Session store
	httpClient   = &http.Client{
		Timeout: 15 * time.Second,
	}
)

func initDB() {
	var err error

	dbHostEnv := os.Getenv("DB_HOST")
	if dbHostEnv == "" {
		dbHostEnv = defaultDBHost
	}
	dbPortEnv := os.Getenv("DB_PORT")
	if dbPortEnv == "" {
		dbPortEnv = defaultDBPort
	}
	dbUserEnv := os.Getenv("DB_USER")
	if dbUserEnv == "" {
		dbUserEnv = defaultDBUser
	}
	dbPasswordEnv := os.Getenv("DB_PASSWORD")
	if dbPasswordEnv == "" {
		dbPasswordEnv = defaultDBPassword
	}
	dbNameEnv := os.Getenv("DB_NAME")
	if dbNameEnv == "" {
		dbNameEnv = defaultDBName
	}
	dbSSLModeEnv := os.Getenv("DB_SSLMODE")
	if dbSSLModeEnv == "" {
		dbSSLModeEnv = defaultSSLMode
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHostEnv, dbPortEnv, dbUserEnv, dbPasswordEnv, dbNameEnv, dbSSLModeEnv)

	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Ping the database to ensure connection is established
	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(255) NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email VARCHAR(255) UNIQUE,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			reset_token TEXT,
			reset_token_expires_at TIMESTAMPTZ,
			is_admin BOOLEAN DEFAULT FALSE,
			avatar_url TEXT
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create users table: %v", err)
	}

	// Migrate existing database for avatar_url
	_, _ = db.Exec("ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT")

	// Create series table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS series (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL,
			title VARCHAR(255) NOT NULL,
			year VARCHAR(20),
			imdb_id VARCHAR(20) NOT NULL,
			episodes_watched INTEGER DEFAULT 0,
			total_episodes INTEGER DEFAULT 0,
			status VARCHAR(50) DEFAULT 'Watching',
			cover_url TEXT,
			UNIQUE (user_id, imdb_id), -- Ensure unique series per user
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create series table: %v", err)
	}

	// Migrate existing database if necessary (increase year column length)
	_, _ = db.Exec("ALTER TABLE series ALTER COLUMN year TYPE VARCHAR(20)")

	// Perform migration if series.json exists and series table is empty
	err = migrateSeriesJSONtoDB()
	if err != nil {
		log.Printf("Migration from series.json failed: %v. Continuing without migration.", err)
	}

	log.Println("Database initialized successfully.")
}

func main() {
	// Initialize database
	initDB()
	defer db.Close()

	// --- MODULE LOAD GUIDANCE ---
	// If you encounter "module not found" or similar errors when running this application,
	// it's likely that Go dependencies haven't been downloaded or synchronized.
	// Please ensure you run the following commands in the project's root directory:
	//
	// go mod tidy
	// go mod download
	//
	// After running these commands, try starting the application again.
	// ----------------------------

	// Initialize session store
	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		log.Fatal("SESSION_SECRET environment variable not set. This is required for secure session management.")
	}
	sessionStore = sessions.NewCookieStore([]byte(sessionSecret))
	sessionStore.Options = &sessions.Options{
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	}

	// Prüfe API-Key zu Start
	if apiKey == "" {
		apiKey = "fbd55d5e" // Fallback to default key if environment variable is not set
		log.Printf("⚠️  WARNUNG: OMDB_API_KEY Umgebungsvariable nicht gesetzt. Nutze Standard-Key.")
	}
	if apiKey == "dein_api_key_hier" || apiKey == "demo" {
		log.Printf("⚠️  WARNUNG: Bitte trage einen gültigen OMDb API-Key in die OMDB_API_KEY Umgebungsvariable ein.")
	}

	// Templates laden
	funcMap := template.FuncMap{
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"percent": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return (a * 100) / b
		},
	}
	templates = template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*.html"))

	// HTTP Routes (protected)
	http.Handle("/", authMiddleware(http.HandlerFunc(indexHandler)))
	http.Handle("/mylist", authMiddleware(http.HandlerFunc(myListHandler)))
	http.Handle("/add", authMiddleware(http.HandlerFunc(addHandler)))
	http.Handle("/update", authMiddleware(http.HandlerFunc(updateHandler)))
	http.Handle("/delete", authMiddleware(http.HandlerFunc(deleteHandler)))
	http.Handle("/search", authMiddleware(http.HandlerFunc(searchHandler)))
	http.Handle("/api/series", authMiddleware(http.HandlerFunc(apiSeriesHandler)))
	http.Handle("/pdf", authMiddleware(http.HandlerFunc(pdfHandler)))
	http.Handle("/stats", authMiddleware(http.HandlerFunc(statsHandler)))
	http.Handle("/admin", authMiddleware(adminMiddleware(http.HandlerFunc(adminHandler))))
	http.Handle("/admin/add-user", authMiddleware(adminMiddleware(http.HandlerFunc(adminAddUserHandler))))
	http.Handle("/admin/reset-password", authMiddleware(adminMiddleware(http.HandlerFunc(adminResetPasswordHandler))))
	http.Handle("/admin/delete-user", authMiddleware(adminMiddleware(http.HandlerFunc(adminDeleteUserHandler))))

	// New Authentication Routes (unprotected)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/forgot-password", forgotPasswordHandler)
	http.HandleFunc("/reset-password", resetPasswordHandler)

	// Statische Dateien
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Automatisch freien Port finden
	port := findAvailablePort()
	if port == 0 {
		port = 8081
	}

	fmt.Printf("🚀 Serien-Tracker Web-Oberfläche läuft auf http://localhost:%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), nil))
}

func findAvailablePort() int {
	for port := 8081; port <= 8090; port++ {
		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port
		}
	}
	return 0
}

// authMiddleware checks for an authenticated user in the session and adds the user object to the request context.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := sessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("Error getting session: %v", err)
			// Continue without a user if session is problematic
			next.ServeHTTP(w, r)
			return
		}

		userID, ok := session.Values[userSessionKey].(int)
		if !ok || userID == 0 {
			// No user in session, or invalid userID
			next.ServeHTTP(w, r)
			return
		}

		user, err := getUserByID(userID)
		if err != nil {
			log.Printf("Error getting user from DB for ID %d: %v", userID, err)
			// User might have been deleted, clear session and continue
			session.Values[userSessionKey] = nil
			session.Save(r, w)
			next.ServeHTTP(w, r)
			return
		}
		if user == nil {
			// User not found, clear session
			session.Values[userSessionKey] = nil
			session.Save(r, w)
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getUserFromContext retrieves the User object from the request context.
func getUserFromContext(ctx context.Context) *User {
	user, ok := ctx.Value(userContextKey).(*User)
	if !ok {
		return nil
	}
	return user
}

// adminMiddleware checks if the authenticated user has admin privileges.
func adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := getUserFromContext(r.Context())
		if user == nil || !user.IsAdmin {
			http.Error(w, "Zugriff verweigert: Nur für Administratoren.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// getUsers returns all registered users from the database.
func getUsers() ([]User, error) {
	rows, err := db.Query("SELECT id, username, email, created_at, is_admin FROM users")
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.IsAdmin); err != nil {
			log.Printf("Error scanning user: %v", err)
			continue
		}
		users = append(users, u)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over users: %w", err)
	}
	return users, nil
}

// getUserByName retrieves a user by their username.
func getUserByName(username string) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, username, password_hash, email, created_at, is_admin FROM users WHERE username = $1", username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.CreatedAt, &user.IsAdmin)
	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user by name: %w", err)
	}
	return &user, nil
}

// getUserByID retrieves a user by their ID.
func getUserByID(id int) (*User, error) {
	var user User
	err := db.QueryRow("SELECT id, username, password_hash, email, created_at, is_admin FROM users WHERE id = $1", id).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email, &user.CreatedAt, &user.IsAdmin)
	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user by ID: %w", err)
	}
	return &user, nil
}

func getUserWithAvatar(u User) UserWithAvatar {
	initial := "U"
	if len(u.Username) > 0 {
		initial = strings.ToUpper(string(u.Username[0]))
	}
	return UserWithAvatar{
		ID:       u.ID,
		Username: u.Username,
		Initial:  initial,
	}
}

func getUserInfos(users []User) []UserWithAvatar {
	infos := make([]UserWithAvatar, len(users))
	for i, u := range users {
		infos[i] = getUserWithAvatar(u)
	}
	return infos
}

// The following functions are removed:
// loadSeries()
// saveSeries()
// getUsers()
// getCurrentUser()

// getAllSeriesForUser retrieves all series for a given user ID from the database.
func getAllSeriesForUser(userID int) ([]Series, error) {
	rows, err := db.Query("SELECT id, title, year, imdb_id, episodes_watched, total_episodes, status, cover_url FROM series WHERE user_id = $1", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query series for user %d: %w", userID, err)
	}
	defer rows.Close()

	var seriesList []Series
	for rows.Next() {
		var s Series
		if err := rows.Scan(&s.ID, &s.Title, &s.Year, &s.IMDBID, &s.EpisodesWatched, &s.TotalEpisodes, &s.Status, &s.CoverURL); err != nil {
			log.Printf("Error scanning series: %v", err)
			continue
		}
		s.UserID = userID // Set UserID for consistency
		if s.TotalEpisodes > 0 {
			s.Progress = (s.EpisodesWatched * 100) / s.TotalEpisodes
		}
		seriesList = append(seriesList, s)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over series: %w", err)
	}
	return seriesList, nil
}

// addSeriesToDB inserts a new series into the database.
func addSeriesToDB(series Series) error {
	stmt := `INSERT INTO series (user_id, title, year, imdb_id, episodes_watched, total_episodes, status, cover_url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`
	var newID int
	err := db.QueryRow(stmt,
		series.UserID, series.Title, series.Year, series.IMDBID, series.EpisodesWatched, series.TotalEpisodes, series.Status, series.CoverURL,
	).Scan(&newID)
	if err != nil {
		return fmt.Errorf("failed to insert series into database: %w", err)
	}
	// The newID is captured but not used directly, as the function returns error.
	// If the calling context needs the ID, the function signature would need to change.
	return nil
}

// updateSeriesInDB updates an existing series in the database.
func updateSeriesInDB(series Series) error {
	res, err := db.Exec(
		"UPDATE series SET episodes_watched = $1, total_episodes = $2, status = $3, cover_url = $4 WHERE id = $5 AND user_id = $6",
		series.EpisodesWatched, series.TotalEpisodes, series.Status, series.CoverURL, series.ID, series.UserID,
	)
	if err != nil {
		return fmt.Errorf("failed to update series in database: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected during update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no series found with ID %d for user %d to update", series.ID, series.UserID)
	}
	return nil
}

// deleteSeriesFromDB deletes a series from the database.
func deleteSeriesFromDB(seriesID int, userID int) error {
	res, err := db.Exec("DELETE FROM series WHERE id = $1 AND user_id = $2", seriesID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete series from database: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected during delete: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("no series found with ID %d for user %d to delete", seriesID, userID)
	}
	return nil
}

// Sortierfunktion
func sortSeries(series []Series, sortBy, order string) {
	switch sortBy {
	case "title":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				return series[i].Title > series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				return series[i].Title < series[j].Title
			})
		}
	case "progress":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				// Höherer Fortschritt zuerst
				if series[i].Progress != series[j].Progress {
					return series[i].Progress > series[j].Progress
				}
				// Bei gleichem Fortschritt: nach Titel sortieren
				return series[i].Title < series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				// Niedriger Fortschritt zuerst
				if series[i].Progress != series[j].Progress {
					return series[i].Progress < series[j].Progress
				}
				return series[i].Title < series[j].Title
			})
		}
	case "watched":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				if series[i].EpisodesWatched != series[j].EpisodesWatched {
					return series[i].EpisodesWatched > series[j].EpisodesWatched
				}
				return series[i].Title < series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				if series[i].EpisodesWatched != series[j].EpisodesWatched {
					return series[i].EpisodesWatched < series[j].EpisodesWatched
				}
				return series[i].Title < series[j].Title
			})
		}
	default:
		sort.Slice(series, func(i, j int) bool {
			return series[i].Title < series[j].Title
		})
	}
}

// HTTP Handler
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	seriesList, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching series: %v", err), http.StatusInternalServerError)
		return
	}

	totalSeries, totalCompleted, err := calculateStats(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error calculating stats: %v", err), http.StatusInternalServerError)
		return
	}

	apiAvailable := testAPIConnection()

	// Get all users for the profile switcher
	allUsers, err := getUsers()
	if err != nil {
		log.Printf("Error getting all users: %v", err)
		allUsers = []User{} // Fallback to empty list
	}

	data := PageData{
		SeriesList:    seriesList,
		APIAvailable:  apiAvailable,
		TotalSeries:   totalSeries,
		TotalWatched:  totalCompleted,
		User:          user,
		CurrentUser:   getUserWithAvatar(*user),
		Users:         getUserInfos(allUsers),
		CurrentUserID: user.ID,
	}

	err = templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func pdfHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	seriesList, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching series for PDF: %v", err), http.StatusInternalServerError)
		return
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	utf8 := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 20)
	pdf.Cell(0, 10, utf8(fmt.Sprintf("Meine Serienliste (%s)", user.Username)))
	pdf.Ln(15)

	countOnPage := 0
	for _, s := range seriesList {
		if countOnPage == 4 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "B", 20)
			pdf.Cell(0, 10, utf8(fmt.Sprintf("Meine Serienliste (%s) (Fortsetzung)", user.Username)))
			pdf.Ln(15)
			countOnPage = 0
		}

		imgWidth := 40.0
		startY := pdf.GetY()
		var imgHeight float64 = 0

		if s.CoverURL != "" && s.CoverURL != "N/A" {
			resp, err := httpClient.Get(s.CoverURL)
			if err == nil {
				func() {
					defer resp.Body.Close()
					data, err := io.ReadAll(resp.Body)
					if err != nil {
						return
					}

					imgName := fmt.Sprintf("cover_%d_%d", user.ID, s.ID) // Unique name per user and series

					info := pdf.RegisterImageOptionsReader(
						imgName,
						gofpdf.ImageOptions{
							ImageType: "JPG",
							ReadDpi:   true,
						},
						bytes.NewReader(data),
					)

					if info != nil && info.Width() > 0 {
						imgHeight = info.Height() * imgWidth / info.Width()

						pdf.ImageOptions(
							imgName,
							10, startY,
							imgWidth, 0,
							false,
							gofpdf.ImageOptions{
								ImageType: "JPG",
								ReadDpi:   true,
							},
							0,
							"",
						)
					}
				}()
			}
		}

		if imgHeight == 0 {
			imgHeight = 20
		}

		textX := 10 + imgWidth + 6
		pdf.SetXY(textX, startY)

		pdf.SetFont("Helvetica", "B", 14)
		pdf.CellFormat(0, 7, utf8(fmt.Sprintf("%s (%s)", s.Title, s.Year)), "", 0, "L", false, 0, "")
		pdf.Ln(8)

		pdf.SetX(textX)
		pdf.SetFont("Helvetica", "", 12)
		pdf.MultiCell(0, 6,
			utf8(fmt.Sprintf("Status: %s – %d/%d Episoden",
				s.Status, s.EpisodesWatched, s.TotalEpisodes)),
			"", "L", false,
		)

		endY := pdf.GetY()
		finalY := startY + imgHeight
		if endY > finalY {
			finalY = endY
		}
		pdf.SetY(finalY + 10)
		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
		pdf.Ln(8)

		countOnPage++
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=mylist_%s.pdf", user.Username))

	err = pdf.Output(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// myListHandler für die Poster-Ansicht
func myListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	seriesList, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching series: %v", err), http.StatusInternalServerError)
		return
	}

	apiAvailable := testAPIConnection()

	sortParam := r.URL.Query().Get("sort")
	var sortBy, order string

	switch sortParam {
	case "title":
		sortBy = "title"
		order = "asc"
	case "title_desc":
		sortBy = "title"
		order = "desc"
	case "progress_asc":
		sortBy = "progress"
		order = "asc"
	case "progress_desc":
		sortBy = "progress"
		order = "desc"
	default:
		sortBy = "title"
		order = "asc"
	}
	sortSeries(seriesList, sortBy, order)

	totalSeries, totalCompleted, err := calculateStats(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error calculating stats: %v", err), http.StatusInternalServerError)
		return
	}

	// Get all users for the profile switcher
	allUsers, err := getUsers()
	if err != nil {
		log.Printf("Error getting all users: %v", err)
		allUsers = []User{} // Fallback to empty list
	}

	data := PageData{
		SeriesList:    seriesList,
		APIAvailable:  apiAvailable,
		TotalSeries:   totalSeries,
		TotalWatched:  totalCompleted, // Note: totalWatched is now totalCompleted
		SortBy:        sortBy,
		Order:         order,
		User:          user,
		CurrentUser:   getUserWithAvatar(*user),
		Users:         getUserInfos(allUsers),
		CurrentUserID: user.ID,
	}

	err = templates.ExecuteTemplate(w, "mylist.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	identifier := r.FormValue("identifier")
	if identifier == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther) // Redirect back to index with an error might be better
		return
	}

	omdbSeries, err := fetchIMDBData(identifier)
	if err != nil {
		seriesList, _ := getAllSeriesForUser(user.ID) // Fetch current user's series to display
		totalSeries, totalCompleted, _ := calculateStats(user.ID)
		allUsers, _ := getUsers()
		data := PageData{
			SeriesList:    seriesList,
			ErrorMessage:  fmt.Sprintf("Fehler beim Hinzufügen: %v", err),
			APIAvailable:  testAPIConnection(),
			TotalSeries:   totalSeries,
			TotalWatched:  totalCompleted,
			User:          user,
			CurrentUser:   getUserWithAvatar(*user),
			Users:         getUserInfos(allUsers),
			CurrentUserID: user.ID,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	totalEpisodes := 0
	if omdbSeries.TotalSeasons != "" {
		if seasons, err := strconv.Atoi(omdbSeries.TotalSeasons); err == nil {
			totalEpisodes = seasons * 10
		}
	}

	// Check if series already exists for this user
	existingSeries, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error checking existing series: %v", err), http.StatusInternalServerError)
		return
	}
	for _, s := range existingSeries {
		if s.IMDBID == omdbSeries.IMDBID {
			seriesList, _ := getAllSeriesForUser(user.ID)
			totalSeries, totalCompleted, _ := calculateStats(user.ID)
			allUsers, _ := getUsers()
			data := PageData{
				SeriesList:    seriesList,
				ErrorMessage:  "Serie ist bereits in deiner Bibliothek",
				APIAvailable:  testAPIConnection(),
				TotalSeries:   totalSeries,
				TotalWatched:  totalCompleted,
				User:          user,
				CurrentUser:   getUserWithAvatar(*user),
				Users:         getUserInfos(allUsers),
				CurrentUserID: user.ID,
			}
			templates.ExecuteTemplate(w, "index.html", data)
			return
		}
	}

	newSeries := Series{
		Title:         omdbSeries.Title,
		Year:          omdbSeries.Year,
		IMDBID:        omdbSeries.IMDBID,
		TotalEpisodes: totalEpisodes,
		Status:        "Watching",
		CoverURL:      omdbSeries.Poster,
		UserID:        user.ID,
	}

	if err := addSeriesToDB(newSeries); err != nil {
		seriesList, _ := getAllSeriesForUser(user.ID)
		totalSeries, totalCompleted, _ := calculateStats(user.ID)
		allUsers, _ := getUsers()
		data := PageData{
			SeriesList:    seriesList,
			ErrorMessage:  fmt.Sprintf("Fehler beim Speichern der Serie: %v", err),
			APIAvailable:  testAPIConnection(),
			TotalSeries:   totalSeries,
			TotalWatched:  totalCompleted,
			User:          user,
			CurrentUser:   getUserWithAvatar(*user),
			Users:         getUserInfos(allUsers),
			CurrentUserID: user.ID,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	// 'Who else watched this series?' logic (simplified for now as users are not globally tracked this way anymore)
	// This would require a separate query across all users.
	// For now, let's just show success message for current user.
	successMsg := fmt.Sprintf("✅ '%s' erfolgreich hinzugefügt!", omdbSeries.Title)

	http.Redirect(w, r, fmt.Sprintf("/?successMessage=%s", url.QueryEscape(successMsg)), http.StatusSeeOther)
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := r.FormValue("id")
	episodesStr := r.FormValue("episodes")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	episodes, err := strconv.Atoi(episodesStr)
	if err != nil {
		http.Error(w, "Invalid episodes number", http.StatusBadRequest)
		return
	}

	// Fetch the series to get its current details
	seriesList, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching series for update: %v", err), http.StatusInternalServerError)
		return
	}

	var foundSeries *Series
	for i, s := range seriesList {
		if s.ID == id {
			foundSeries = &seriesList[i]
			break
		}
	}

	if foundSeries == nil {
		http.Error(w, "Series not found or does not belong to user", http.StatusNotFound)
		return
	}

	foundSeries.EpisodesWatched = episodes
	// Optionally update status if all episodes watched
	if foundSeries.TotalEpisodes > 0 && episodes >= foundSeries.TotalEpisodes {
		foundSeries.Status = "Completed"
	} else {
		foundSeries.Status = "Watching"
	}

	if err := updateSeriesInDB(*foundSeries); err != nil {
		http.Error(w, fmt.Sprintf("Error updating series: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := deleteSeriesFromDB(id, user.ID); err != nil {
		http.Error(w, fmt.Sprintf("Error deleting series: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	results, err := searchIMDBData(query)
	if err != nil {
		seriesList, _ := getAllSeriesForUser(user.ID)
		totalSeries, totalCompleted, _ := calculateStats(user.ID)
		allUsers, _ := getUsers()
		data := PageData{
			SeriesList:    seriesList,
			SearchQuery:   query,
			ErrorMessage:  fmt.Sprintf("Suche fehlgeschlagen: %v", err),
			APIAvailable:  testAPIConnection(),
			TotalSeries:   totalSeries,
			TotalWatched:  totalCompleted,
			User:          user,
			CurrentUser:   getUserWithAvatar(*user),
			Users:         getUserInfos(allUsers),
			CurrentUserID: user.ID,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	var seriesResults []SearchItem
	for _, item := range results.Search {
		if item.Type == "series" {
			seriesResults = append(seriesResults, item)
		}
	}

	seriesList, _ := getAllSeriesForUser(user.ID)
	totalSeries, totalCompleted, _ := calculateStats(user.ID)
	allUsers, _ := getUsers()
	data := PageData{
		SeriesList:    seriesList,
		SearchResults: seriesResults,
		SearchQuery:   query,
		APIAvailable:  testAPIConnection(),
		TotalSeries:   totalSeries,
		TotalWatched:  totalCompleted,
		User:          user,
		CurrentUser:   getUserWithAvatar(*user),
		Users:         getUserInfos(allUsers),
		CurrentUserID: user.ID,
	}

	if len(seriesResults) == 0 && len(results.Search) > 0 {
		data.ErrorMessage = "Keine Serien gefunden (nur Filme oder andere Typen)"
	} else if len(seriesResults) == 0 {
		data.ErrorMessage = "Keine Ergebnisse gefunden"
	}

	err = templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// SortierHandler (falls genutzt) - updated to use session-based user
func seriesHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	seriesList, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching series: %v", err), http.StatusInternalServerError)
		return
	}

	sortBy := r.URL.Query().Get("sort")
	order := "asc" // Standard

	switch sortBy {
	case "title":
		sortBy = "title"
		order = "asc"
	case "title_desc":
		sortBy = "title"
		order = "desc"
	case "progress_asc":
		sortBy = "progress"
		order = "asc"
	case "progress_desc":
		sortBy = "progress"
		order = "desc"
	default:
		sortBy = "title"
		order = "asc"
	}

	sortSeries(seriesList, sortBy, order)

	totalSeries, totalCompleted, err := calculateStats(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error calculating stats: %v", err), http.StatusInternalServerError)
		return
	}

	allUsers, err := getUsers()
	if err != nil {
		log.Printf("Error getting all users: %v", err)
		allUsers = []User{} // Fallback to empty list
	}

	data := PageData{
		SeriesList:    seriesList,
		TotalSeries:   totalSeries,
		TotalWatched:  totalCompleted,
		SortBy:        sortBy,
		Order:         order,
		User:          user,
		CurrentUser:   getUserWithAvatar(*user),
		Users:         getUserInfos(allUsers),
		CurrentUserID: user.ID,
	}

	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, data)
}

// apiSeriesHandler
func apiSeriesHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	seriesList, err := getAllSeriesForUser(user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching series: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(seriesList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// sendResetEmail sends a password reset email to the user.
func sendResetEmail(userEmail, username, token string, r *http.Request) error {
	resetLink := fmt.Sprintf("http://%s/reset-password?token=%s", r.Host, token)

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "localhost" // Default for local development
	}
	smtpPort := os.Getenv("SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "1025" // Default MailHog SMTP port
	}

	from := mail.Address{Name: "Serien Tracker", Address: "no-reply@serientracker.de"}
	to := mail.Address{Name: username, Address: userEmail}

	headers := make(map[string]string)
	headers["From"] = from.String()
	headers["To"] = to.String()
	headers["Subject"] = "Passwort zurücksetzen für Serien Tracker"
	headers["MIME-version"] = "1.0"
	headers["Content-Type"] = "text/plain; charset=\"UTF-8\""

	body := fmt.Sprintf(`Hallo %s,

Sie haben eine Anfrage zum Zurücksetzen Ihres Passworts für Ihr Serien Tracker-Konto gestellt.

Bitte klicken Sie auf den folgenden Link, um Ihr Passwort zurückzusetzen:
%s

Dieser Link ist 1 Stunde lang gültig.

Wenn Sie diese Anfrage nicht gestellt haben, ignorieren Sie diese E-Mail einfach.

Mit freundlichen Grüßen,
Ihr Serien Tracker Team`, username, resetLink)

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	// Connect to the SMTP server
	conn, err := smtp.Dial(smtpHost + ":" + smtpPort)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Set the sender and recipient
	if err = conn.Mail(from.Address); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}
	if err = conn.Rcpt(to.Address); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send the email body.
	wc, err := conn.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}
	_, err = io.WriteString(wc, message)
	if err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}
	err = wc.Close()
	if err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	// Send the QUIT command and close the connection.
	err = conn.Quit()
	if err != nil {
		return fmt.Errorf("failed to quit SMTP connection: %w", err)
	}

	log.Printf("Password reset email sent to %s for user %s. Link: %s", userEmail, username, resetLink)
	return nil
}

// generateResetToken generates a secure, random token.
func generateResetToken() (string, error) {
	b := make([]byte, 32) // 32 bytes for a 64-character token
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Hilfsfunktionen

func testAPIConnection() bool {
	// Einfacher Test mit einer bekannten Serie
	testURL := fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&t=Game%%20of%%20Thrones&r=json", apiKey)
	resp, err := httpClient.Get(testURL)
	if err != nil {
		log.Printf("API Verbindungstest fehlgeschlagen: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("API Verbindungstest: Status %d", resp.StatusCode)
		return false
	}

	var result struct {
		Response string `json:"Response"`
		Error    string `json:"Error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("API Verbindungstest: JSON Fehler %v", err)
		return false
	}

	return result.Response != "False"
}

// calculateStats calculates total series and total completed series for a given user.
func calculateStats(userID int) (int, int, error) {
	seriesList, err := getAllSeriesForUser(userID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get series for stats: %w", err)
	}

	totalSeries := len(seriesList)
	totalCompleted := 0

	for _, s := range seriesList {
		if s.TotalEpisodes > 0 && s.EpisodesWatched == s.TotalEpisodes {
			totalCompleted++
		}
	}

	return totalSeries, totalCompleted, nil
}

func fetchIMDBData(identifier string) (*OMDbResponse, error) {
	baseURL := "http://www.omdbapi.com/"

	params := url.Values{}
	params.Add("apikey", apiKey)
	params.Add("r", "json")

	if len(identifier) > 2 && identifier[:2] == "tt" {
		params.Add("i", identifier)
	} else {
		params.Add("t", url.QueryEscape(identifier))
		params.Add("type", "series")
	}

	url := baseURL + "?" + params.Encode()

	log.Printf("📡 API Aufruf: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Netzwerkfehler: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("API Key ungültig oder abgelaufen (Status 401)")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API antwortet mit Status: %d", resp.StatusCode)
	}

	var result OMDbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Fehler beim Lesen der Antwort: %v", err)
	}

	if result.Response == "False" {
		if result.Error != "" {
			return nil, fmt.Errorf("API Fehler: %s", result.Error)
		}
		return nil, fmt.Errorf("Serie nicht gefunden")
	}

	return &result, nil
}

func searchIMDBData(query string) (*SearchResult, error) {
	baseURL := "http://www.omdbapi.com/"

	params := url.Values{}
	params.Add("apikey", apiKey)
	params.Add("s", url.QueryEscape(query))
	params.Add("type", "series")
	params.Add("r", "json")
	params.Add("page", "1")

	url := baseURL + "?" + params.Encode()

	log.Printf("🔍 Such-API Aufruf: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Netzwerkfehler: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("API Key ungültig oder abgelaufen (Status 401)")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API antwortet mit Status: %d", resp.StatusCode)
	}

	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Fehler beim Lesen der Antwort: %v", err)
	}

	if result.Response == "False" {
		if result.Error != "" {
			return nil, fmt.Errorf("API Fehler: %s", result.Error)
		}
		return nil, fmt.Errorf("Keine Ergebnisse gefunden")
	}

	return &result, nil
}

func updateMissingCovers() {
	updated := false

	// Fetch all series from the database
	rows, err := db.Query("SELECT id, user_id, title, imdb_id, cover_url FROM series")
	if err != nil {
		log.Printf("Error fetching all series for cover update: %v", err)
		return
	}
	defer rows.Close()

	type seriesCoverInfo struct {
		ID       int
		UserID   int
		Title    string
		IMDBID   string
		CoverURL string
	}
	var allSeries []seriesCoverInfo

	for rows.Next() {
		var s seriesCoverInfo
		if err := rows.Scan(&s.ID, &s.UserID, &s.Title, &s.IMDBID, &s.CoverURL); err != nil {
			log.Printf("Error scanning series for cover update: %v", err)
			continue
		}
		allSeries = append(allSeries, s)
	}
	if err = rows.Err(); err != nil {
		log.Printf("Error iterating over series for cover update: %v", err)
		return
	}

	for _, s := range allSeries {
		// Überspringen, wenn bereits ein Cover existiert
		if s.CoverURL != "" && s.CoverURL != "N/A" {
			continue
		}

		// IMDb-ID muss vorhanden sein
		if s.IMDBID == "" {
			continue
		}

		log.Printf("📥 Lade Cover für %s (%s) nach...", s.Title, s.IMDBID)

		// API Aufruf
		data, err := fetchIMDBData(s.IMDBID)
		if err != nil {
			log.Printf("❌ Fehler beim Nachladen eines Covers: %v", err)
			continue
		}

		// Prüfen ob Poster existiert
		if data.Poster != "" && data.Poster != "N/A" {
			_, err := db.Exec("UPDATE series SET cover_url = $1 WHERE id = $2", data.Poster, s.ID)
			if err != nil {
				log.Printf("Error updating cover for series %d: %v", s.ID, err)
			} else {
				updated = true
				log.Printf("✅ Cover gespeichert: %s", data.Poster)
			}
		}
	}

	if updated {
		log.Println("✔️ Cover erfolgreich aktualisiert!")
	} else {
		log.Println("ℹ️ Keine fehlenden Cover gefunden.")
	}
}
func statsHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser := getUserFromContext(r.Context())
	if loggedInUser == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	allUsers, err := getUsers()
	if err != nil {
		log.Printf("Error getting all users for stats: %v", err)
		http.Error(w, "Error fetching user stats", http.StatusInternalServerError)
		return
	}

	var stats []UserStatsData
	for _, u := range allUsers {
		seriesList, err := getAllSeriesForUser(u.ID)
		if err != nil {
			log.Printf("Error fetching series for user %s: %v", u.Username, err)
			continue
		}

		episodesWatched := 0
		episodesTotal := 0
		completed := 0

		for _, s := range seriesList {
			episodesWatched += s.EpisodesWatched
			episodesTotal += s.TotalEpisodes
			if s.TotalEpisodes > 0 && s.EpisodesWatched == s.TotalEpisodes {
				completed++
			}
		}

		progress := 0
		if episodesTotal > 0 {
			progress = (episodesWatched * 100) / episodesTotal
		}

		stats = append(stats, UserStatsData{
			User:            u.Username,
			EpisodesWatched: episodesWatched,
			EpisodesTotal:   episodesTotal,
			Progress:        progress,
			Completed:       completed,
			WatchTimeHours:  (episodesWatched * 45) / 60,
		})
	}

	// Sort by episodes watched to determine rank
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].EpisodesWatched > stats[j].EpisodesWatched
	})

	// Assign ranks and find current user data
	var currentUserStats UserStatsData
	for i := range stats {
		stats[i].Rank = i + 1
		if stats[i].User == loggedInUser.Username {
			currentUserStats = stats[i]
		}
	}

	data := PageData{
		UserStats:      stats,
		User:           loggedInUser,
		CurrentUser:    getUserWithAvatar(*loggedInUser),
		Users:          getUserInfos(allUsers),
		CurrentUserID:  loggedInUser.ID,
		TotalSeries:    currentUserStats.Completed, // Using it for 'Completed' in hero
		TotalWatched:   currentUserStats.EpisodesWatched,
		TotalEpisodes:  currentUserStats.EpisodesTotal,
		WatchTimeHours: currentUserStats.WatchTimeHours,
		Rank:           currentUserStats.Rank,
	}

	err = templates.ExecuteTemplate(w, "stats.html", data)
	if err != nil {
		log.Printf("Error executing stats template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// adminHandler renders the admin dashboard.
func adminHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser := getUserFromContext(r.Context())
	allUsers, err := getUsers()
	if err != nil {
		log.Printf("Error getting all users for admin: %v", err)
		http.Error(w, "Error fetching user data", http.StatusInternalServerError)
		return
	}

	data := PageData{
		User:          loggedInUser,
		CurrentUser:   getUserWithAvatar(*loggedInUser),
		Users:         getUserInfos(allUsers),
		FullUsers:     allUsers,
		CurrentUserID: loggedInUser.ID,
	}

	err = templates.ExecuteTemplate(w, "admin.html", data)
	if err != nil {
		log.Printf("Error executing admin template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// adminAddUserHandler handles manual user creation by an admin.
func adminAddUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	email := r.FormValue("email")
	isAdmin := r.FormValue("is_admin") == "on"

	if username == "" || password == "" || email == "" {
		http.Error(w, "Alle Felder sind Pflichtfelder", http.StatusBadRequest)
		return
	}

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	_, err := db.Exec("INSERT INTO users (username, password_hash, email, is_admin) VALUES ($1, $2, $3, $4)",
		username, string(hashedPassword), email, isAdmin)

	if err != nil {
		log.Printf("Error admin adding user: %v", err)
		http.Error(w, "Fehler beim Erstellen des Nutzers", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?success=added", http.StatusSeeOther)
}

// adminResetPasswordHandler allows an admin to force-reset a user's password.
func adminResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetUserIDStr := r.FormValue("user_id")
	newPassword := r.FormValue("new_password")

	targetUserID, _ := strconv.Atoi(targetUserIDStr)
	if targetUserID == 0 || newPassword == "" {
		http.Error(w, "Ungültige Daten", http.StatusBadRequest)
		return
	}

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	_, err := db.Exec("UPDATE users SET password_hash = $1 WHERE id = $2", string(hashedPassword), targetUserID)

	if err != nil {
		log.Printf("Error admin resetting password: %v", err)
		http.Error(w, "Fehler beim Zurücksetzen des Passworts", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?success=reset", http.StatusSeeOther)
}

// adminDeleteUserHandler handles user deletion by an admin.
func adminDeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetUserIDStr := r.FormValue("user_id")
	targetUserID, _ := strconv.Atoi(targetUserIDStr)

	loggedInUser := getUserFromContext(r.Context())
	if loggedInUser.ID == targetUserID {
		http.Error(w, "Man kann sich nicht selbst löschen!", http.StatusBadRequest)
		return
	}

	// Delete series first (foreign key should handle this if configured, but let's be safe)
	db.Exec("DELETE FROM series WHERE user_id = $1", targetUserID)
	_, err := db.Exec("DELETE FROM users WHERE id = $1", targetUserID)

	if err != nil {
		log.Printf("Error admin deleting user: %v", err)
		http.Error(w, "Fehler beim Löschen des Nutzers", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?success=deleted", http.StatusSeeOther)
}

// loginHandler renders the login form or handles login submission.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		successMessage := r.URL.Query().Get("successMessage")
		templates.ExecuteTemplate(w, "login.html", PageData{SuccessMessage: successMessage})
		return
	}

	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")

		user, err := getUserByName(username)
		if err != nil {
			log.Printf("Error during login for user %s: %v", username, err)
			templates.ExecuteTemplate(w, "login.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		if user == nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			templates.ExecuteTemplate(w, "login.html", PageData{ErrorMessage: "Ungültiger Benutzername oder Passwort."})
			return
		}

		session, err := sessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("Error getting session for user %s: %v", username, err)
			templates.ExecuteTemplate(w, "login.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		session.Values[userSessionKey] = user.ID
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// registerHandler renders the registration form or handles registration submission.
func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "register.html", nil)
		return
	}

	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")
		email := r.FormValue("email")

		// Basic validation
		if username == "" || password == "" || email == "" {
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Alle Felder müssen ausgefüllt werden."})
			return
		}

		// Check if username or email already exists
		existingUser, err := getUserByName(username)
		if err != nil {
			log.Printf("Error checking for existing user %s: %v", username, err)
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}
		if existingUser != nil {
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Benutzername existiert bereits."})
			return
		}

		// Hash password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing password for user %s: %v", username, err)
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		var isFirstUser bool
		var userCount int
		err = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
		if err != nil {
			log.Printf("Error checking user count: %v", err)
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}
		if userCount == 0 {
			isFirstUser = true
		}

		// Insert new user into DB
		var newUserID int
		err = db.QueryRow("INSERT INTO users (username, password_hash, email, is_admin) VALUES ($1, $2, $3, $4) RETURNING id", username, string(hashedPassword), email, isFirstUser).Scan(&newUserID)
		if err != nil {
			log.Printf("Error creating new user %s: %v", username, err)
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		// Set session for new user
		session, err := sessionStore.Get(r, sessionName)
		if err != nil {
			log.Printf("Error getting session after registration for user %s: %v", username, err)
			templates.ExecuteTemplate(w, "register.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}
		session.Values[userSessionKey] = newUserID // Use newUserID
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// logoutHandler clears the user's session and redirects to the login page.
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := sessionStore.Get(r, sessionName)
	if err != nil {
		log.Printf("Error getting session for logout: %v", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	session.Values[userSessionKey] = nil // Clear user ID from session
	session.Options.MaxAge = -1          // Expire cookie
	session.Save(r, w)

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// forgotPasswordHandler renders the forgot password form or handles email submission.
func forgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "forgot_password.html", nil)
		return
	}

	if r.Method == "POST" {
		email := r.FormValue("email")
		if email == "" {
			templates.ExecuteTemplate(w, "forgot_password.html", PageData{ErrorMessage: "Bitte geben Sie Ihre E-Mail-Adresse ein."})
			return
		}

		var user User
		err := db.QueryRow("SELECT id, username, email FROM users WHERE email = $1", email).Scan(&user.ID, &user.Username, &user.Email)
		if err == sql.ErrNoRows {
			// Do not reveal if email exists for security reasons
			log.Printf("Password reset requested for non-existent email: %s", email)
			templates.ExecuteTemplate(w, "forgot_password.html", PageData{SuccessMessage: "Wenn die E-Mail-Adresse in unserem System existiert, haben Sie einen Link zum Zurücksetzen des Passworts erhalten."})
			return
		}
		if err != nil {
			log.Printf("Error querying user for password reset: %v", err)
			templates.ExecuteTemplate(w, "forgot_password.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		token, err := generateResetToken()
		if err != nil {
			log.Printf("Error generating reset token: %v", err)
			templates.ExecuteTemplate(w, "forgot_password.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		expiresAt := time.Now().Add(1 * time.Hour) // Token valid for 1 hour

		_, err = db.Exec("UPDATE users SET reset_token = $1, reset_token_expires_at = $2 WHERE id = $3", token, expiresAt, user.ID)
		if err != nil {
			log.Printf("Error saving reset token to DB for user %d: %v", user.ID, err)
			templates.ExecuteTemplate(w, "forgot_password.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten."})
			return
		}

		err = sendResetEmail(user.Email, user.Username, token, r)
		if err != nil {
			log.Printf("Error sending reset email to %s: %v", user.Email, err)
			templates.ExecuteTemplate(w, "forgot_password.html", PageData{ErrorMessage: "Fehler beim Senden der E-Mail zum Zurücksetzen des Passworts."})
			return
		}

		templates.ExecuteTemplate(w, "forgot_password.html", PageData{SuccessMessage: "Wenn die E-Mail-Adresse in unserem System existiert, haben Sie einen Link zum Zurücksetzen des Passworts erhalten."})
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// resetPasswordHandler validates the token and allows the user to set a new password.
func resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Invalid reset token", http.StatusBadRequest)
		return
	}

	var user User
	var expiresAt time.Time
	err := db.QueryRow("SELECT id, username, email, reset_token_expires_at FROM users WHERE reset_token = $1", token).Scan(&user.ID, &user.Username, &user.Email, &expiresAt)
	if err == sql.ErrNoRows {
		templates.ExecuteTemplate(w, "reset_password.html", PageData{ErrorMessage: "Ungültiger oder abgelaufener Token."})
		return
	}
	if err != nil {
		log.Printf("Error querying user by reset token: %v", err)
		http.Error(w, "Ein Fehler ist aufgetreten.", http.StatusInternalServerError)
		return
	}

	if time.Now().After(expiresAt) {
		templates.ExecuteTemplate(w, "reset_password.html", PageData{ErrorMessage: "Der Token ist abgelaufen. Bitte fordern Sie einen neuen Link an."})
		return
	}

	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "reset_password.html", PageData{SearchQuery: token}) // Use SearchQuery to pass token to template
		return
	}

	if r.Method == "POST" {
		newPassword := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")

		if newPassword == "" || newPassword != confirmPassword {
			templates.ExecuteTemplate(w, "reset_password.html", PageData{ErrorMessage: "Passwörter stimmen nicht überein oder sind leer.", SearchQuery: token})
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing new password for user %d: %v", user.ID, err)
			templates.ExecuteTemplate(w, "reset_password.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten.", SearchQuery: token})
			return
		}

		_, err = db.Exec("UPDATE users SET password_hash = $1, reset_token = NULL, reset_token_expires_at = NULL WHERE id = $2", string(hashedPassword), user.ID)
		if err != nil {
			log.Printf("Error updating password for user %d: %v", user.ID, err)
			templates.ExecuteTemplate(w, "reset_password.html", PageData{ErrorMessage: "Ein Fehler ist aufgetreten.", SearchQuery: token})
			return
		}

		http.Redirect(w, r, "/login?successMessage="+url.QueryEscape("Passwort erfolgreich zurückgesetzt. Sie können sich jetzt anmelden."), http.StatusSeeOther)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// migrateSeriesJSONtoDB reads series.json and inserts data into the SQLite database.
func migrateSeriesJSONtoDB() error {
	// Check if series.json exists
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		log.Println("series.json not found, skipping migration.")
		return nil // No file, no migration needed
	}

	// Check if series table is empty
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM series").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check series table count: %w", err)
	}
	if count > 0 {
		log.Println("Series table is not empty, skipping migration from series.json.")
		return nil // Table already has data, assuming it's already migrated or intentionally empty
	}

	data, err := os.ReadFile(dataFile)
	if err != nil {
		return fmt.Errorf("failed to read series.json: %w", err)
	}

	var oldSeriesList []OldSeries
	if err := json.Unmarshal(data, &oldSeriesList); err != nil {
		return fmt.Errorf("failed to unmarshal series.json: %w", err)
	}

	// Ensure a default user "Dan" exists
	var danUserID int
	err = db.QueryRow("SELECT id FROM users WHERE username = $1", "Dan").Scan(&danUserID)
	if err == sql.ErrNoRows {
		// User "Dan" does not exist, create him
		hashedPassword, hashErr := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost) // Use a temporary default password
		if hashErr != nil {
			return fmt.Errorf("failed to hash password for default user Dan: %w", hashErr)
		}
		res, execErr := db.Exec("INSERT INTO users (username, password_hash, email) VALUES ($1, $2, $3)", "Dan", string(hashedPassword), "dan@example.com")
		if execErr != nil {
			return fmt.Errorf("failed to create default user Dan: %w", execErr)
		}
		id, lastIdErr := res.LastInsertId()
		if lastIdErr != nil {
			return fmt.Errorf("failed to get last insert ID for default user Dan: %w", lastIdErr)
		}
		danUserID = int(id)
		log.Println("Default user 'Dan' created for migration.")
	} else if err != nil {
		return fmt.Errorf("failed to query for default user Dan: %w", err)
	}
	log.Printf("Migrating %d series from series.json to database for user Dan (ID: %d)...", len(oldSeriesList), danUserID)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for migration: %w", err)
	}
	defer tx.Rollback() // The rollback will be ignored if the transaction is committed

	stmt, err := tx.Prepare(`
		INSERT INTO series (user_id, title, year, imdb_id, episodes_watched, total_episodes, status, cover_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement for migration: %w", err)
	}
	defer stmt.Close()

	for _, s := range oldSeriesList {
		_, err := stmt.Exec(
			danUserID,
			s.Title,
			s.Year,
			s.IMDBID,
			s.EpisodesWatched,
			s.TotalEpisodes,
			s.Status,
			s.CoverURL,
		)
		if err != nil {
			log.Printf("Warning: Failed to insert series '%s' (IMDBID: %s) during migration: %v", s.Title, s.IMDBID, err)
			// Continue with next series even if one fails
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	// Delete series.json after successful migration
	if err := os.Remove(dataFile); err != nil {
		log.Printf("Warning: Failed to remove series.json after migration: %v", err)
	} else {
		log.Println("series.json successfully migrated and removed.")
	}

	return nil
}
=======
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sort"
	"sync"
	"time"

	"github.com/jung-kurt/gofpdf"
)

type Series struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	Year            string `json:"year"`
	IMDBID          string `json:"imdb_id"`
	EpisodesWatched int    `json:"episodes_watched"`
	TotalEpisodes   int    `json:"total_episodes"`
	Status          string `json:"status"`
	Progress        int    `json:"progress"`
	CoverURL        string `db:"cover_url"` // Für SQLX oder ähnliche ORMs
}

type OMDbResponse struct {
	Title        string `json:"Title"`
	Year         string `json:"Year"`
	TotalSeasons string `json:"totalSeasons"`
	IMDBID       string `json:"imdbID"`
	Response     string `json:"Response"`
	Error        string `json:"Error"`
	Poster       string `json:"Poster"`
}

type SearchResult struct {
	Search       []SearchItem `json:"Search"`
	Response     string       `json:"Response"`
	Error        string       `json:"Error"`
	TotalResults string       `json:"totalResults"`
}

type SearchItem struct {
	Title  string `json:"Title"`
	Year   string `json:"Year"`
	IMDBID string `json:"imdbID"`
	Type   string `json:"Type"`
	Poster string `json:"Poster"`
}

type PageData struct {
	SeriesList     []Series
	SearchResults  []SearchItem
	SearchQuery    string
	ErrorMessage   string
	SuccessMessage string
	APIAvailable   bool
	TotalSeries    int
	TotalWatched   int
	SortBy         string // z. B. "title", "progress"
	Order          string // "asc" oder "desc"
}

// TRAG DEINEN API-KEY HIER EIN
const (
	apiKey  = "dein_api_key_hier"  //<<<------------------ DEIN API KEY HIER REIN
	dataFile = "series.json"
	dbPath  = "series.db"
)
var (
	templates  *template.Template
	seriesDB   []Series
	mutex      sync.Mutex
	nextID     = 1
	httpClient = &http.Client{
		Timeout: 15 * time.Second,
	}
)

func main() {
	// Prüfe API-Key zu Start
	if apiKey == "dein_api_key_hier" || apiKey == "demo" {
		log.Printf("⚠️  WARNUNG: Bitte trage deinen echten OMDb API-Key in die main.go ein")
	}

	// Daten laden
	loadSeries()
	// Fehlende Cover automatisch nachladen
	updateMissingCovers()
	// Templates laden
	templates = template.Must(template.ParseGlob("templates/*.html"))

	// HTTP Routes
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/mylist", myListHandler)
	http.HandleFunc("/add", addHandler)
	http.HandleFunc("/update", updateHandler)
	http.HandleFunc("/delete", deleteHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/api/series", apiSeriesHandler)
	http.HandleFunc("/pdf", pdfHandler)

	// Statische Dateien
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Automatisch freien Port finden
	port := findAvailablePort()
	if port == 0 {
		port = 8081
	}

	fmt.Printf("🚀 Serien-Tracker Web-Oberfläche läuft auf http://localhost:%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func findAvailablePort() int {
	for port := 8080; port <= 8090; port++ {
		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port
		}
	}
	return 0
}

func loadSeries() {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		seriesDB = []Series{}
		return
	}

	err = json.Unmarshal(data, &seriesDB)
	if err != nil {
		log.Printf("Fehler beim Laden der Daten: %v", err)
		seriesDB = []Series{}
	}

	for _, s := range seriesDB {
		if s.ID >= nextID {
			nextID = s.ID + 1
		}
	}
}

func saveSeries() {
	mutex.Lock()
	defer mutex.Unlock()

	data, err := json.MarshalIndent(seriesDB, "", "  ")
	if err != nil {
		log.Printf("Fehler beim Speichern: %v", err)
		return
	}

	err = os.WriteFile(dataFile, data, 0644)
	if err != nil {
		log.Printf("Fehler beim Schreiben der Datei: %v", err)
	}
}
//Sortierfunktion
func sortSeries(series []Series, sortBy, order string) {
	switch sortBy {
	case "title":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				return series[i].Title > series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				return series[i].Title < series[j].Title
			})
		}
	case "progress":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				// Höherer Fortschritt zuerst
				if series[i].Progress != series[j].Progress {
					return series[i].Progress > series[j].Progress
				}
				// Bei gleichem Fortschritt: nach Titel sortieren (optional für Stabilität)
				return series[i].Title < series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				// Niedriger Fortschritt zuerst
				if series[i].Progress != series[j].Progress {
					return series[i].Progress < series[j].Progress
				}
				return series[i].Title < series[j].Title
			})
		}
	// Optional: Sortierung nach "EpisodesWatched"
	case "watched":
		if order == "desc" {
			sort.Slice(series, func(i, j int) bool {
				if series[i].EpisodesWatched != series[j].EpisodesWatched {
					return series[i].EpisodesWatched > series[j].EpisodesWatched
				}
				return series[i].Title < series[j].Title
			})
		} else {
			sort.Slice(series, func(i, j int) bool {
				if series[i].EpisodesWatched != series[j].EpisodesWatched {
					return series[i].EpisodesWatched < series[j].EpisodesWatched
				}
				return series[i].Title < series[j].Title
			})
		}
	default:
		// Standard: nach Titel aufsteigend
		sort.Slice(series, func(i, j int) bool {
			return series[i].Title < series[j].Title
		})
	}
}
// HTTP Handler
func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	series := getAllSeries()
	totalSeries, totalWatched := calculateStats(series)
	apiAvailable := testAPIConnection()

	data := PageData{
		SeriesList:   series,
		APIAvailable: apiAvailable,
		TotalSeries:  totalSeries,
		TotalWatched: totalWatched,
	}

	err := templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func pdfHandler(w http.ResponseWriter, r *http.Request) {
	pdf := gofpdf.New("P", "mm", "A4", "")

	// Standard-Font laden
	pdf.SetFont("Helvetica", "", 12)

	// UTF-8 Übersetzer anlegen (kompatibel mit vielen gofpdf-Versionen)
	utf8 := pdf.UnicodeTranslatorFromDescriptor("")

	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 20)
	pdf.Cell(0, 10, utf8("Meine Serienliste"))
	pdf.Ln(15)

	series := getAllSeries()
	countOnPage := 0

	for _, s := range series {

	    // Wenn bereits 4 Serien auf der Seite → neue Seite
	    if countOnPage == 4 {
	        pdf.AddPage()
	        pdf.SetFont("Helvetica", "B", 20)
	        pdf.Cell(0, 10, utf8("Meine Serienliste (Fortsetzung)"))
	        pdf.Ln(15)
	        countOnPage = 0
	    }

	    // === Linke Spalte: Cover =====================================
	    imgWidth := 40.0
	    startY := pdf.GetY()
	    var imgHeight float64 = 0

	    if s.CoverURL != "" && s.CoverURL != "N/A" {
	        resp, err := httpClient.Get(s.CoverURL)
	        if err == nil {
	            func() {
	                defer resp.Body.Close()
	                data, err := io.ReadAll(resp.Body)
	                if err != nil {
	                    return
	                }

	                imgName := fmt.Sprintf("cover_%d", s.ID)

	                info := pdf.RegisterImageOptionsReader(
	                    imgName,
	                    gofpdf.ImageOptions{
	                        ImageType: "JPG",
	                        ReadDpi:   true,
	                    },
	                    bytes.NewReader(data),
	                )

	                if info != nil && info.Width() > 0 {
	                    imgHeight = info.Height() * imgWidth / info.Width()

	                    pdf.ImageOptions(
	                        imgName,
	                        10, startY,
	                        imgWidth, 0,
	                        false,
	                        gofpdf.ImageOptions{
	                            ImageType: "JPG",
	                            ReadDpi:   true,
	                        },
	                        0,
	                        "",
	                    )
	                }
	            }()
	        }
	    }

	    if imgHeight == 0 {
	        imgHeight = 20
	    }

	    // === Rechte Spalte: Text =====================================
	    textX := 10 + imgWidth + 6
	    pdf.SetXY(textX, startY)

	    pdf.SetFont("Helvetica", "B", 14)
	    pdf.CellFormat(0, 7, utf8(fmt.Sprintf("%s (%s)", s.Title, s.Year)), "", 0, "L", false, 0, "")
	    pdf.Ln(8)

	    pdf.SetX(textX)
	    pdf.SetFont("Helvetica", "", 12)
	    pdf.MultiCell(0, 6,
	        utf8(fmt.Sprintf("Status: %s – %d/%d Episoden",
	            s.Status, s.EpisodesWatched, s.TotalEpisodes)),
	        "", "L", false,
	    )

	    // Höhe bestimmen
	    endY := pdf.GetY()
	    finalY := startY + imgHeight
	    if endY > finalY {
	        finalY = endY
	    }

	    // Abstand zum nächsten Block
	    pdf.SetY(finalY + 10)

	    // Trennlinie
	    pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	    pdf.Ln(8)

	    // Erhöhe Counter für die Seite
	    countOnPage++
	}


	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=mylist.pdf")

	err := pdf.Output(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// NEUE FUNKTION: myListHandler für die Poster-Ansicht
func myListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	series := getAllSeries()
	//totalSeries, totalWatched := calculateStats(series)
	apiAvailable := testAPIConnection()
	sortParam := r.URL.Query().Get("sort")
		var sortBy, order string

		switch sortParam {
		case "title":
			sortBy = "title"
			order = "asc"
		case "title_desc":
			sortBy = "title"
			order = "desc"
		case "progress_asc":
			sortBy = "progress"
			order = "asc"
		case "progress_desc":
			sortBy = "progress"
			order = "desc"
		default:
			sortBy = "title"
			order = "asc"
		}
  sortSeries(series, sortBy, order)

	totalSeries := len(series)
	totalWatched := 0
	for _, s := range series {
		totalWatched += s.EpisodesWatched
	}

	data := PageData{
		SeriesList:   series,
		APIAvailable: apiAvailable,
		TotalSeries:  totalSeries,
		TotalWatched: totalWatched,
		SortBy:         sortBy,
    Order:          order,
	}

	err := templates.ExecuteTemplate(w, "mylist.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identifier := r.FormValue("identifier")
	if identifier == "" {
		http.Error(w, "Identifier required", http.StatusBadRequest)
		return
	}

	series, err := fetchIMDBData(identifier)
	if err != nil {
		seriesList := getAllSeries()
		totalSeries, totalWatched := calculateStats(seriesList)
		data := PageData{
			SeriesList:   seriesList,
			ErrorMessage: fmt.Sprintf("Fehler beim Hinzufügen: %v", err),
			APIAvailable: testAPIConnection(),
			TotalSeries:  totalSeries,
			TotalWatched: totalWatched,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	totalEpisodes := 0
	if series.TotalSeasons != "" {
		if seasons, err := strconv.Atoi(series.TotalSeasons); err == nil {
			totalEpisodes = seasons * 10
		}
	}

	// Prüfen ob Serie bereits existiert
	for _, s := range seriesDB {
		if s.IMDBID == series.IMDBID {
			seriesList := getAllSeries()
			totalSeries, totalWatched := calculateStats(seriesList)
			data := PageData{
				SeriesList:   seriesList,
				ErrorMessage: "Serie ist bereits in deiner Bibliothek",
				APIAvailable: testAPIConnection(),
				TotalSeries:  totalSeries,
				TotalWatched: totalWatched,
			}
			templates.ExecuteTemplate(w, "index.html", data)
			return
		}
	}

	// Neue Serie hinzufügen
	newSeries := Series{
		ID:             nextID,
		Title:          series.Title,
		Year:           series.Year,
		IMDBID:         series.IMDBID,
		TotalEpisodes:  totalEpisodes,
		Status:         "Watching",
		CoverURL:       series.Poster,
	}
	nextID++

	mutex.Lock()
	seriesDB = append(seriesDB, newSeries)
	mutex.Unlock()

	saveSeries()

	seriesList := getAllSeries()
	totalSeries, totalWatched := calculateStats(seriesList)
	data := PageData{
		SeriesList:     seriesList,
		SuccessMessage: fmt.Sprintf("✅ '%s' erfolgreich hinzugefügt!", series.Title),
		APIAvailable:   testAPIConnection(),
		TotalSeries:    totalSeries,
		TotalWatched:   totalWatched,
	}
	templates.ExecuteTemplate(w, "index.html", data)
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	episodesStr := r.FormValue("episodes")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	episodes, err := strconv.Atoi(episodesStr)
	if err != nil {
		http.Error(w, "Invalid episodes number", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	found := false
	for i := range seriesDB {
		if seriesDB[i].ID == id {
			seriesDB[i].EpisodesWatched = episodes
			found = true
			break
		}
	}
	mutex.Unlock()

	if !found {
		http.Error(w, "Series not found", http.StatusNotFound)
		return
	}

	saveSeries()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	newSeries := []Series{}
	for _, s := range seriesDB {
		if s.ID != id {
			newSeries = append(newSeries, s)
		}
	}
	seriesDB = newSeries
	mutex.Unlock()

	saveSeries()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	results, err := searchIMDBData(query)
	if err != nil {
		seriesList := getAllSeries()
		totalSeries, totalWatched := calculateStats(seriesList)
		data := PageData{
			SeriesList:    seriesList,
			SearchQuery:   query,
			ErrorMessage:  fmt.Sprintf("Suche fehlgeschlagen: %v", err),
			APIAvailable:  testAPIConnection(),
			TotalSeries:   totalSeries,
			TotalWatched:  totalWatched,
		}
		templates.ExecuteTemplate(w, "index.html", data)
		return
	}

	// Filtere nur Serien
	var seriesResults []SearchItem
	for _, item := range results.Search {
		if item.Type == "series" {
			seriesResults = append(seriesResults, item)
		}
	}

	seriesList := getAllSeries()
	totalSeries, totalWatched := calculateStats(seriesList)
	data := PageData{
		SeriesList:     seriesList,
		SearchResults:  seriesResults,
		SearchQuery:    query,
		APIAvailable:   testAPIConnection(),
		TotalSeries:    totalSeries,
		TotalWatched:   totalWatched,
	}

	if len(seriesResults) == 0 && len(results.Search) > 0 {
		data.ErrorMessage = "Keine Serien gefunden (nur Filme oder andere Typen)"
	} else if len(seriesResults) == 0 {
		data.ErrorMessage = "Keine Ergebnisse gefunden"
	}

	err = templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
//SortierHandler
func seriesHandler(w http.ResponseWriter, r *http.Request) {
	// Hole alle Serien aus deinem Speicher (z. B. JSON, DB, etc.)
	seriesList := getAllSeries() // ← deine Funktion

	// Hole Sortierparameter aus der URL (optional)
	sortBy := r.URL.Query().Get("sort")
	order := "asc" // Standard

	// Interpretiere das Dropdown-Format
	switch sortBy {
	case "title":
		sortBy = "title"
		order = "asc"
	case "title_desc":
		sortBy = "title"
		order = "desc"
	case "progress_asc":
		sortBy = "progress"
		order = "asc"
	case "progress_desc":
		sortBy = "progress"
		order = "desc"
	default:
		sortBy = "title"
		order = "asc"
	}

	// Sortiere!
	sortSeries(seriesList, sortBy, order)

	// Berechne ggf. Statistiken
	totalSeries := len(seriesList)
	totalWatched := 0
	for _, s := range seriesList {
		totalWatched += s.EpisodesWatched
	}

	data := PageData{
		SeriesList:   seriesList,
		TotalSeries:  totalSeries,
		TotalWatched: totalWatched,
		SortBy:       sortBy,
		Order:        order,
		// ... ggf. andere Felder
	}

	// Template rendern
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, data)
}
//apiSeriesHandler
func apiSeriesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(getAllSeries())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Hilfsfunktionen
func getAllSeries() []Series {
	mutex.Lock()
	defer mutex.Unlock()

	result := make([]Series, len(seriesDB))
	for i, s := range seriesDB {
		s.Progress = 0
		if s.TotalEpisodes > 0 {
			s.Progress = (s.EpisodesWatched * 100) / s.TotalEpisodes
		}
		result[i] = s
	}

	return result
}

func testAPIConnection() bool {
	// Einfacher Test mit einer bekannten Serie
	testURL := fmt.Sprintf("http://www.omdbapi.com/?apikey=%s&t=Game%%20of%%20Thrones&r=json", apiKey)
	resp, err := httpClient.Get(testURL)
	if err != nil {
		log.Printf("API Verbindungstest fehlgeschlagen: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("API Verbindungstest: Status %d", resp.StatusCode)
		return false
	}

	var result struct {
		Response string `json:"Response"`
		Error    string `json:"Error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("API Verbindungstest: JSON Fehler %v", err)
		return false
	}

	return result.Response != "False"
}

func calculateStats(series []Series) (int, int) {
	totalSeries := len(series)
	totalWatched := 0

	for _, s := range series {
		if s.Progress == 100 {
			totalWatched++
		}
	}

	return totalSeries, totalWatched
}

func fetchIMDBData(identifier string) (*OMDbResponse, error) {
	baseURL := "http://www.omdbapi.com/"

	params := url.Values{}
	params.Add("apikey", apiKey)
	params.Add("r", "json")

	if len(identifier) > 2 && identifier[:2] == "tt" {
		params.Add("i", identifier)
	} else {
		params.Add("t", url.QueryEscape(identifier))
		params.Add("type", "series")
	}

	url := baseURL + "?" + params.Encode()

	log.Printf("📡 API Aufruf: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Netzwerkfehler: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("API Key ungültig oder abgelaufen (Status 401)")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API antwortet mit Status: %d", resp.StatusCode)
	}

	var result OMDbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Fehler beim Lesen der Antwort: %v", err)
	}

	if result.Response == "False" {
		if result.Error != "" {
			return nil, fmt.Errorf("API Fehler: %s", result.Error)
		}
		return nil, fmt.Errorf("Serie nicht gefunden")
	}

	return &result, nil
}

func searchIMDBData(query string) (*SearchResult, error) {
	baseURL := "http://www.omdbapi.com/"

	params := url.Values{}
	params.Add("apikey", apiKey)
	params.Add("s", url.QueryEscape(query))
	params.Add("type", "series")
	params.Add("r", "json")
	params.Add("page", "1")

	url := baseURL + "?" + params.Encode()

	log.Printf("🔍 Such-API Aufruf: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("Netzwerkfehler: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("API Key ungültig oder abgelaufen (Status 401)")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API antwortet mit Status: %d", resp.StatusCode)
	}

	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("Fehler beim Lesen der Antwort: %v", err)
	}

	if result.Response == "False" {
		if result.Error != "" {
			return nil, fmt.Errorf("API Fehler: %s", result.Error)
		}
		return nil, fmt.Errorf("Keine Ergebnisse gefunden")
	}

	return &result, nil
}
func updateMissingCovers() {
    updated := false

    for i, s := range seriesDB {
        // Überspringen, wenn bereits ein Cover existiert
        if s.CoverURL != "" && s.CoverURL != "N/A" {
            continue
        }

        // IMDb-ID muss vorhanden sein
        if s.IMDBID == "" {
            continue
        }

        log.Printf("📥 Lade Cover für %s (%s) nach...", s.Title, s.IMDBID)

        // API Aufruf
        data, err := fetchIMDBData(s.IMDBID)
        if err != nil {
            log.Printf("❌ Fehler beim Nachladen eines Covers: %v", err)
            continue
        }

        // Prüfen ob Poster existiert
        if data.Poster != "" && data.Poster != "N/A" {
            seriesDB[i].CoverURL = data.Poster
            updated = true
            log.Printf("✅ Cover gespeichert: %s", data.Poster)
        }
    }

    if updated {
        log.Println("💾 Speichere aktualisierte Serien-Datenbank...")
        saveSeries()
        log.Println("✔️ Cover erfolgreich aktualisiert!")
    } else {
        log.Println("ℹ️ Keine fehlenden Cover gefunden.")
    }
}
>>>>>>> 4174669acaf558a2cb39d52e11b7963c62aec47e
