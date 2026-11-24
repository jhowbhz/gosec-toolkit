package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Tipos de rota descobertos.
const (
	kindPage     = "page"     // página navegável do mesmo host (segue)
	kindXHR      = "xhr"      // chamada de API que a página fez (endpoint real)
	kindAsset    = "asset"    // js/css/img (registra, não segue)
	kindExternal = "external" // outra origem (registra, não segue)
)

// route é uma rota/recurso descoberto no crawl.
type route struct {
	URL       string    `json:"url"`
	Host      string    `json:"host"`
	Method    string    `json:"method"`
	Status    int       `json:"status"`
	Kind      string    `json:"kind"`
	SourceURL string    `json:"source_url"`
	Depth     int       `json:"depth"`
	Title     string    `json:"title"`
	FirstSeen time.Time `json:"first_seen"`
}

// store persiste as rotas em SQLite, deduplicando por (url, método).
type store struct {
	db *sql.DB
}

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Uma conexão só: evita contenção de escrita do SQLite e mantém a ordem.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db}, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS routes (
  url        TEXT NOT NULL,
  host       TEXT,
  method     TEXT NOT NULL DEFAULT 'GET',
  status     INTEGER,
  kind       TEXT,
  source_url TEXT,
  depth      INTEGER,
  title      TEXT,
  first_seen TEXT,
  PRIMARY KEY (url, method)
);`

// salvar insere a rota se ela for nova (dedup por url+método). Devolve true
// quando era inédita — é o gatilho pra emitir o evento ao vivo.
func (s *store) salvar(r route) (bool, error) {
	if r.Method == "" {
		r.Method = "GET"
	}
	if r.FirstSeen.IsZero() {
		r.FirstSeen = time.Now().UTC()
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO routes
		 (url, host, method, status, kind, source_url, depth, title, first_seen)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.URL, r.Host, r.Method, r.Status, r.Kind, r.SourceURL, r.Depth, r.Title,
		r.FirstSeen.Format(time.RFC3339),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// listar devolve todas as rotas na ordem em que foram vistas.
func (s *store) listar() ([]route, error) {
	rows, err := s.db.Query(
		`SELECT url, host, method, status, kind, source_url, depth, title, first_seen
		 FROM routes ORDER BY first_seen, url`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []route
	for rows.Next() {
		var r route
		var firstSeen string
		if err := rows.Scan(&r.URL, &r.Host, &r.Method, &r.Status, &r.Kind,
			&r.SourceURL, &r.Depth, &r.Title, &firstSeen); err != nil {
			return nil, err
		}
		r.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *store) close() error {
	return s.db.Close()
}
