package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Installation struct {
	ProfileToken            string     `json:"profile_token"`
	Stage2Token             string     `json:"stage2_token"`
	ClientIdentifier        string     `json:"client_identifier"`
	SelectedLatitude        *float64   `json:"selected_latitude,omitempty"`
	SelectedLongitude       *float64   `json:"selected_longitude,omitempty"`
	SelectedLabel           string     `json:"selected_label"`
	LocationRevision        int64      `json:"location_revision"`
	DoHSeenAt               *time.Time `json:"doh_seen_at,omitempty"`
	ProxySeenAt             *time.Time `json:"proxy_seen_at,omitempty"`
	Stage2DeliveredAt       *time.Time `json:"stage2_delivered_at,omitempty"`
	Stage2DeliveryCount     int64      `json:"stage2_delivery_count"`
	IdentityEnrolledAt      *time.Time `json:"identity_enrolled_at,omitempty"`
	IdentityCertFingerprint string     `json:"identity_cert_fingerprint"`
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
	stage2_token TEXT NOT NULL DEFAULT '',
	client_identifier TEXT NOT NULL DEFAULT '',
	selected_latitude REAL,
	selected_longitude REAL,
	selected_label TEXT NOT NULL DEFAULT '',
	location_revision INTEGER NOT NULL DEFAULT 0,
	doh_seen_at TEXT,
	proxy_seen_at TEXT,
	stage2_delivered_at TEXT,
	stage2_delivery_count INTEGER NOT NULL DEFAULT 0,
	identity_enrolled_at TEXT,
	identity_cert_fingerprint TEXT NOT NULL DEFAULT ''
);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Existing installations predate the two-stage enrollment columns. These
	// ALTERs are intentionally additive so selected location/dashboard state is
	// preserved in place.
	for _, migration := range []string{
		`ALTER TABLE installation ADD COLUMN stage2_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE installation ADD COLUMN client_identifier TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE installation ADD COLUMN stage2_delivered_at TEXT`,
		`ALTER TABLE installation ADD COLUMN stage2_delivery_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE installation ADD COLUMN identity_enrolled_at TEXT`,
		`ALTER TABLE installation ADD COLUMN identity_cert_fingerprint TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(migration); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate schema: %w", err)
		}
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS acme_account (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	account_key_thumbprint TEXT NOT NULL DEFAULT '',
	account_jwk TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS acme_order (
	id TEXT PRIMARY KEY,
	account_id INTEGER NOT NULL DEFAULT 1,
	client_identifier TEXT NOT NULL,
	status TEXT NOT NULL,
	challenge_token TEXT NOT NULL DEFAULT '',
	challenge_token_hash TEXT NOT NULL,
	challenge_status TEXT NOT NULL,
	attested_spki_sha256 TEXT NOT NULL DEFAULT '',
	csr_sha256 TEXT NOT NULL DEFAULT '',
	certificate_serial TEXT NOT NULL DEFAULT '',
	certificate_pem BLOB,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL,
	finalized_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS acme_order_client_identifier
ON acme_order(client_identifier);
CREATE TABLE IF NOT EXISTS acme_nonce (
	nonce_hash TEXT PRIMARY KEY,
	expires_at TEXT NOT NULL,
	consumed_at TEXT
);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create enrollment schema: %w", err)
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS location_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	label TEXT NOT NULL DEFAULT '',
	revision INTEGER NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS location_history_revision
ON location_history(revision DESC);
CREATE TABLE IF NOT EXISTS location_preset (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	label TEXT NOT NULL,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS location_preset_unique
ON location_preset(label COLLATE NOCASE, latitude, longitude);
CREATE TRIGGER IF NOT EXISTS installation_location_history
AFTER UPDATE OF location_revision ON installation
WHEN NEW.location_revision > OLD.location_revision
BEGIN
	INSERT INTO location_history(latitude, longitude, label, revision, created_at)
	VALUES(
		NEW.selected_latitude,
		NEW.selected_longitude,
		NEW.selected_label,
		NEW.location_revision,
		strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	);
END;
`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create location dashboard schema: %w", err)
	}

	for _, migration := range []string{
		`ALTER TABLE acme_account ADD COLUMN account_jwk TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE acme_order ADD COLUMN account_id INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE acme_order ADD COLUMN challenge_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE acme_order ADD COLUMN attested_spki_sha256 TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE acme_order ADD COLUMN certificate_pem BLOB`,
	} {
		if _, err := db.Exec(migration); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate ACME schema: %w", err)
		}
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

// EnsureEnrollmentCredentials initializes the new enrollment generation exactly
// once. On an upgraded database this intentionally rotates the old DoH token,
// while preserving target coordinates, label, revision, and traffic history.
func (s *Store) EnsureEnrollmentCredentials(profileToken, stage2Token, clientIdentifier string) error {
	if profileToken == "" || stage2Token == "" || clientIdentifier == "" {
		return errors.New("all enrollment credentials are required")
	}
	result, err := s.db.Exec(`
UPDATE installation
SET profile_token = ?, stage2_token = ?, client_identifier = ?
WHERE id = 1 AND (stage2_token = '' OR client_identifier = '')`,
		profileToken, stage2Token, clientIdentifier)
	if err != nil {
		return fmt.Errorf("ensure enrollment credentials: %w", err)
	}
	if _, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("enrollment credential rows affected: %w", err)
	}
	return nil
}

func (s *Store) Installation() (Installation, error) {
	row := s.db.QueryRow(`
SELECT profile_token, stage2_token, client_identifier,
       selected_latitude, selected_longitude, selected_label, location_revision,
       doh_seen_at, proxy_seen_at, stage2_delivered_at, stage2_delivery_count,
       identity_enrolled_at, identity_cert_fingerprint
FROM installation WHERE id = 1`)
	var inst Installation
	var lat, lon sql.NullFloat64
	var doh, proxy, stage2, identity sql.NullString
	if err := row.Scan(
		&inst.ProfileToken,
		&inst.Stage2Token,
		&inst.ClientIdentifier,
		&lat,
		&lon,
		&inst.SelectedLabel,
		&inst.LocationRevision,
		&doh,
		&proxy,
		&stage2,
		&inst.Stage2DeliveryCount,
		&identity,
		&inst.IdentityCertFingerprint,
	); err != nil {
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
	var err error
	if inst.DoHSeenAt, err = parseOptionalTime(doh); err != nil {
		return Installation{}, fmt.Errorf("parse doh timestamp: %w", err)
	}
	if inst.ProxySeenAt, err = parseOptionalTime(proxy); err != nil {
		return Installation{}, fmt.Errorf("parse proxy timestamp: %w", err)
	}
	if inst.Stage2DeliveredAt, err = parseOptionalTime(stage2); err != nil {
		return Installation{}, fmt.Errorf("parse stage2 timestamp: %w", err)
	}
	if inst.IdentityEnrolledAt, err = parseOptionalTime(identity); err != nil {
		return Installation{}, fmt.Errorf("parse identity timestamp: %w", err)
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

func (s *Store) MarkStage2Delivered(at time.Time) error {
	result, err := s.db.Exec(`
UPDATE installation
SET stage2_delivered_at = ?, stage2_delivery_count = stage2_delivery_count + 1
WHERE id = 1`, at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("mark stage 2 delivered: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark stage 2 rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("installation is not initialized")
	}
	return nil
}

func (s *Store) MarkIdentityEnrolled(at time.Time, certFingerprint string) error {
	if strings.TrimSpace(certFingerprint) == "" {
		return errors.New("identity certificate fingerprint is required")
	}
	result, err := s.db.Exec(`
UPDATE installation
SET identity_enrolled_at = ?, identity_cert_fingerprint = ?
WHERE id = 1`, at.UTC().Format(time.RFC3339Nano), certFingerprint)
	if err != nil {
		return fmt.Errorf("mark identity enrolled: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark identity rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("installation is not initialized")
	}
	return nil
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

func parseOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

