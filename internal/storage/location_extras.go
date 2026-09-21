package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type LocationHistoryEntry struct {
	ID        int64     `json:"id"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Label     string    `json:"label"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
}

type LocationPreset struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) LocationHistory(limit int) ([]LocationHistoryEntry, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.Query(`
SELECT id, latitude, longitude, label, revision, created_at
FROM location_history
ORDER BY revision DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query location history: %w", err)
	}
	defer rows.Close()

	var out []LocationHistoryEntry
	for rows.Next() {
		var item LocationHistoryEntry
		var created string
		if err := rows.Scan(&item.ID, &item.Latitude, &item.Longitude, &item.Label, &item.Revision, &created); err != nil {
			return nil, fmt.Errorf("scan location history: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse location history timestamp: %w", err)
		}
		item.CreatedAt = t
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate location history: %w", err)
	}
	return out, nil
}

func (s *Store) LocationPresets() ([]LocationPreset, error) {
	rows, err := s.db.Query(`
SELECT id, label, latitude, longitude, created_at
FROM location_preset
ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query location presets: %w", err)
	}
	defer rows.Close()

	var out []LocationPreset
	for rows.Next() {
		var item LocationPreset
		var created string
		if err := rows.Scan(&item.ID, &item.Label, &item.Latitude, &item.Longitude, &created); err != nil {
			return nil, fmt.Errorf("scan location preset: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse location preset timestamp: %w", err)
		}
		item.CreatedAt = t
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate location presets: %w", err)
	}
	return out, nil
}

func (s *Store) SaveLocationPreset(label string, lat, lon float64) (LocationPreset, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return LocationPreset{}, errors.New("preset label is required")
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return LocationPreset{}, errors.New("preset coordinates out of range")
	}
	now := time.Now().UTC()
	result, err := s.db.Exec(`
INSERT INTO location_preset(label, latitude, longitude, created_at)
VALUES(?, ?, ?, ?)`,
		label, lat, lon, now.Format(time.RFC3339Nano))
	if err != nil {
		return LocationPreset{}, fmt.Errorf("save location preset: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return LocationPreset{}, fmt.Errorf("read location preset id: %w", err)
	}
	return LocationPreset{
		ID:        id,
		Label:     label,
		Latitude:  lat,
		Longitude: lon,
		CreatedAt: now,
	}, nil
}

func (s *Store) DeleteLocationPreset(id int64) error {
	if id <= 0 {
		return errors.New("preset id is required")
	}
	result, err := s.db.Exec(`DELETE FROM location_preset WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete location preset: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete location preset rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
