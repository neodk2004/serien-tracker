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
	totalSeries, totalWatched := calculateStats(series)
	apiAvailable := testAPIConnection()

	data := PageData{
		SeriesList:   series,
		APIAvailable: apiAvailable,
		TotalSeries:  totalSeries,
		TotalWatched: totalWatched,
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
