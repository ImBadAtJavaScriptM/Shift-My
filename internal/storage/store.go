package storage

import (
    "database/sql"
    "errors"
    "fmt"
    "time"

    _ "modernc.org/sqlite"
)

type Store struct {
    db *sql.DB
}

type Installation struct {
    ProfileToken      string     `json:"profile_token"`
    SelectedLatitude  *float64   `json:"selected_latitude,omitempty"`
    SelectedLongitude *float64   `json:"selected_longitude,omitempty"`
    SelectedLabel      string     `json:"selected_label"`
    LocationRevision  int64      `json:"location_revision"`
    DoHSeenAt          *time.Time `json:"doh_seen_at,omitempty"`
    ProxySeenAt        *time.Time `json:"proxy_seen_at,omitempty"`
}

func Open(path string) (*Store, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("open sqlite: %w", err)
    }
    if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS installation (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    profile_token TEXT NOT NULL,
    selected_latitude REAL,
    selected_longitude REAL,
    selected_label TEXT NOT NULL DEFAULT '',
    location_revision INTEGER NOT NULL DEFAULT 0,
    doh_seen_at TEXT,
    proxy_seen_at TEXT
);`); err != nil {
        db.Close()
        return nil, fmt.Errorf("create schema: %w", err)
    }
    return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) EnsureInstallation(token string) error {
    if token == "" {
        return errors.New("profile token is required")
    }
    _, err := s.db.Exec(`INSERT OR IGNORE INTO installation (id, profile_token) VALUES (1, ?)`, token)
    if err != nil {
        return fmt.Errorf("ensure installation: %w", err)
    }
    return nil
}

func (s *Store) Installation() (Installation, error) {
    row := s.db.QueryRow(`SELECT profile_token, selected_latitude, selected_longitude, selected_label, location_revision, doh_seen_at, proxy_seen_at FROM installation WHERE id = 1`)
    var inst Installation
    var lat, lon sql.NullFloat64
    var doh, proxy sql.NullString
    if err := row.Scan(&inst.ProfileToken, &lat, &lon, &inst.SelectedLabel, &inst.LocationRevision, &doh, &proxy); err != nil {
        return Installation{}, fmt.Errorf("read installation: %w", err)
    }
    if lat.Valid {
        v := lat.Float64
        inst.SelectedLatitude = &v
    }
    if lon.Valid {
        v := lon.Float64
        inst.SelectedLongitude = &v
    }
    if doh.Valid {
        t, err := time.Parse(time.RFC3339Nano, doh.String)
        if err != nil {
            return Installation{}, fmt.Errorf("parse doh timestamp: %w", err)
        }
        inst.DoHSeenAt = &t
    }
    if proxy.Valid {
        t, err := time.Parse(time.RFC3339Nano, proxy.String)
        if err != nil {
            return Installation{}, fmt.Errorf("parse proxy timestamp: %w", err)
        }
        inst.ProxySeenAt = &t
    }
    return inst, nil
}

func (s *Store) UpdateTarget(lat, lon float64, label string) (Installation, error) {
    if lat < -90 || lat > 90 {
        return Installation{}, fmt.Errorf("latitude out of range")
    }
    if lon < -180 || lon > 180 {
        return Installation{}, fmt.Errorf("longitude out of range")
    }
    tx, err := s.db.Begin()
    if err != nil {
        return Installation{}, fmt.Errorf("begin target update: %w", err)
    }
    defer tx.Rollback()
    result, err := tx.Exec(`UPDATE installation SET selected_latitude = ?, selected_longitude = ?, selected_label = ?, location_revision = location_revision + 1 WHERE id = 1`, lat, lon, label)
    if err != nil {
        return Installation{}, fmt.Errorf("update target: %w", err)
    }
    rows, err := result.RowsAffected()
    if err != nil {
        return Installation{}, fmt.Errorf("target rows affected: %w", err)
    }
    if rows != 1 {
        return Installation{}, errors.New("installation is not initialized")
    }
    if err := tx.Commit(); err != nil {
        return Installation{}, fmt.Errorf("commit target update: %w", err)
    }
    return s.Installation()
}

func (s *Store) MarkDoHSeen(at time.Time) error {
    return s.markSeen("doh_seen_at", at)
}

func (s *Store) MarkProxySeen(at time.Time) error {
    return s.markSeen("proxy_seen_at", at)
}

func (s *Store) markSeen(column string, at time.Time) error {
    var query string
    switch column {
    case "doh_seen_at":
        query = `UPDATE installation SET doh_seen_at = ? WHERE id = 1`
    case "proxy_seen_at":
        query = `UPDATE installation SET proxy_seen_at = ? WHERE id = 1`
    default:
        return fmt.Errorf("unsupported seen column %q", column)
    }
    result, err := s.db.Exec(query, at.UTC().Format(time.RFC3339Nano))
    if err != nil {
        return fmt.Errorf("mark %s: %w", column, err)
    }
    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("mark %s rows affected: %w", column, err)
    }
    if rows != 1 {
        return errors.New("installation is not initialized")
    }
    return nil
}
