use anyhow::{anyhow, Result};
use rusqlite::{params, Connection};
use serde::Deserialize;
use std::env;

#[derive(Debug)]
struct Series {
    id: i32,
    title: String,
    year: String,
    imdb_id: String,
    episodes_watched: i32,
    total_episodes: i32,
    status: String,
}

#[derive(Deserialize, Debug)]
#[allow(non_snake_case)]
struct OMDbResponse {
    Title: String,
    Year: String,
    totalSeasons: Option<String>,
    imdbID: String,
    Response: String,
}

fn main() -> Result<()> {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        print_help();
        return Ok(());
    }

    let conn = setup_database()?;

    match args[1].as_str() {
        "add" => add_series(&conn, &args[2..]),
        "list" => list_series(&conn),
        "update" => update_progress(&conn, &args[2..]),
        "search" => search_imdb(&args[2]),
        _ => {
            print_help();
            Ok(())
        }
    }
}

fn setup_database() -> Result<Connection> {
    let conn = Connection::open("series.db")?;
    conn.execute(
        "CREATE TABLE IF NOT EXISTS series (
            id INTEGER PRIMARY KEY,
            title TEXT NOT NULL,
            year TEXT NOT NULL,
            imdb_id TEXT UNIQUE NOT NULL,
            episodes_watched INTEGER DEFAULT 0,
            total_episodes INTEGER DEFAULT 0,
            status TEXT DEFAULT 'Watching'
        )",
        [],
    )?;
    Ok(conn)
}

fn add_series(conn: &Connection, args: &[String]) -> Result<()> {
    if args.is_empty() {
        return Err(anyhow!("Usage: add <imdb_id|title>"));
    }

    let input = &args[0];
    let response = fetch_imdb_data(input)?;

    let total_episodes_estimate = response.totalSeasons
        .unwrap_or_else(|| "0".to_string())
        .parse::<i32>()
        .unwrap_or(0) * 10;

    conn.execute(
        "INSERT OR IGNORE INTO series (title, year, imdb_id, total_episodes)
         VALUES (?1, ?2, ?3, ?4)",
        params![
            response.Title,
            response.Year,
            response.imdbID,
            total_episodes_estimate
        ],
    )?;

    println!("✅ '{}' zur Bibliothek hinzugefügt", response.Title);
    Ok(())
}

fn list_series(conn: &Connection) -> Result<()> {
    let mut stmt = conn.prepare(
        "SELECT id, title, year, imdb_id, episodes_watched, total_episodes, status
         FROM series ORDER BY status, title",
    )?;

    let series_iter = stmt.query_map([], |row| {
        Ok(Series {
            id: row.get(0)?,
            title: row.get(1)?,
            year: row.get(2)?,
            imdb_id: row.get(3)?,
            episodes_watched: row.get(4)?,
            total_episodes: row.get(5)?,
            status: row.get(6)?,
        })
    })?;

    println!("\n🎬 Deine Serien-Bibliothek:");
    println!("ID | Titel (Jahr) | Fortschritt | Status");
    println!("----------------------------------------");

    for series in series_iter {
        let s = series?;
        let progress = if s.total_episodes > 0 {
            (s.episodes_watched as f32 / s.total_episodes as f32 * 100.0) as i32
        } else {
            0
        };

        println!(
            "{} | {} ({}) | {}/{} ({}%) | {}",
            s.id, s.title, s.year, s.episodes_watched, s.total_episodes, progress, s.status
        );
    }
    Ok(())
}

fn update_progress(conn: &Connection, args: &[String]) -> Result<()> {
    if args.len() < 2 {
        return Err(anyhow!("Usage: update <id> <episodes_watched>"));
    }

    let id: i32 = args[0].parse()?;
    let episodes: i32 = args[1].parse()?;

    let rows_affected = conn.execute(
        "UPDATE series SET episodes_watched = ?1 WHERE id = ?2",
        params![episodes, id],
    )?;

    if rows_affected == 0 {
        return Err(anyhow!("Serie mit ID {} nicht gefunden", id));
    }

    println!("✅ Fortschritt aktualisiert");
    Ok(())
}

fn search_imdb(query: &str) -> Result<()> {
    let response = search_imdb_data(query)?;
    println!("\n🔍 Suchergebnisse für '{}':", query);
    for item in response.Search {
        println!(
            "▸ {} ({}) - https://www.imdb.com/title/{}",
            item.Title, item.Year, item.imdbID
        );
    }
    Ok(())
}

fn fetch_imdb_data(identifier: &str) -> Result<OMDbResponse> {
    let api_key = "55570c3a";
    let url = if identifier.starts_with("tt") {
        format!("http://www.omdbapi.com/?i={}&apikey={}", identifier, api_key)
    } else {
        format!("http://www.omdbapi.com/?t={}&apikey={}", identifier, api_key)
    };

    let response = minreq::get(&url)
        .send()?
        .json::<OMDbResponse>()?;

    if response.Response == "False" {
        return Err(anyhow!("Serie nicht gefunden"));
    }
    Ok(response)
}

#[derive(Deserialize)]
#[allow(non_snake_case)]
struct SearchResult {
    Search: Vec<SearchItem>,
}

#[derive(Deserialize)]
#[allow(non_snake_case)]
struct SearchItem {
    Title: String,
    Year: String,
    imdbID: String,
}

fn search_imdb_data(query: &str) -> Result<SearchResult> {
    let api_key = "55570c3a";
    let url = format!("http://www.omdbapi.com/?s={}&apikey={}", query, api_key);
    let response = minreq::get(&url)
        .send()?
        .json::<SearchResult>()?;
    Ok(response)
}

fn print_help() {
    println!(
        r#"
🎬 Serien-Tracker CLI

Nutzung:
  cargo run -- add <IMDb-ID oder Titel>   Serie hinzufügen
  cargo run -- list                        Bibliothek anzeigen
  cargo run -- update <id> <episoden>     Fortschritt aktualisieren
  cargo run -- search <titel>             Bei IMDb suchen

Beispiele:
  cargo run -- add "Breaking Bad"
  cargo run -- add tt0903747
  cargo run -- update 1 5
  cargo run -- search "Game of Thrones"
    "#
    );
}
