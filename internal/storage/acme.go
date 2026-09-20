package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrACMENotFound = errors.New("ACME record not found")
var ErrACMEConflict = errors.New("ACME record conflict")

type ACMEAccount struct {
	ID         int64
	Thumbprint string
	JWK        string
	Status     string
	CreatedAt  time.Time
}

type ACMEOrder struct {
	ID                   string
	AccountID            int64
	ClientIdentifier     string
	Status               string
	ChallengeToken       string
	ChallengeTokenHash   string
	ChallengeStatus      string
	AttestedSPKISHA256   string
	CSRSHA256            string
	CertificateSerial    string
	CertificatePEM       []byte
	ExpiresAt            time.Time
	CreatedAt            time.Time
	FinalizedAt          *time.Time
}

func (s *Store) PutACMENonce(value string, expiresAt time.Time) error {
	if value == "" {
		return errors.New("ACME nonce is required")
	}
	sum := sha256.Sum256([]byte(value))
	_, err := s.db.Exec(
		`INSERT INTO acme_nonce (nonce_hash, expires_at, consumed_at) VALUES (?, ?, NULL)`,
		hex.EncodeToString(sum[:]),
		expiresAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("store ACME nonce: %w", err)
	}
	return nil
}

func (s *Store) ConsumeACMENonce(value string, now time.Time) (bool, error) {
	if value == "" {
		return false, nil
	}
	sum := sha256.Sum256([]byte(value))
	result, err := s.db.Exec(
		`UPDATE acme_nonce
SET consumed_at = ?
WHERE nonce_hash = ? AND consumed_at IS NULL AND expires_at >= ?`,
		now.UTC().Format(time.RFC3339Nano),
		hex.EncodeToString(sum[:]),
		now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("consume ACME nonce: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("consume ACME nonce rows affected: %w", err)
	}
	return rows == 1, nil
}

func (s *Store) ACMEAccount() (ACMEAccount, error) {
	var a ACMEAccount
	var created string
	err := s.db.QueryRow(
		`SELECT id, account_key_thumbprint, account_jwk, status, created_at
FROM acme_account WHERE id = 1`,
	).Scan(&a.ID, &a.Thumbprint, &a.JWK, &a.Status, &created)
	if err != nil {
		if errors.Is(err, sqlErrNoRows()) {
			return ACMEAccount{}, ErrACMENotFound
		}
		return ACMEAccount{}, fmt.Errorf("read ACME account: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return ACMEAccount{}, fmt.Errorf("parse ACME account timestamp: %w", err)
	}
	a.CreatedAt = t
	return a, nil
}

// EnsureACMEAccount creates the one account supported by this single-iPhone
// deployment. Re-registering the same key returns the existing account; a
// different key is rejected until enrollment is explicitly reset.
func (s *Store) EnsureACMEAccount(thumbprint, jwk string, at time.Time) (ACMEAccount, bool, error) {
	if thumbprint == "" || jwk == "" {
		return ACMEAccount{}, false, errors.New("ACME account key is required")
	}
	current, err := s.ACMEAccount()
	if err == nil {
		if current.Thumbprint != thumbprint {
			return ACMEAccount{}, false, ErrACMEConflict
		}
		return current, false, nil
	}
	if !errors.Is(err, ErrACMENotFound) {
		return ACMEAccount{}, false, err
	}
	_, err = s.db.Exec(
		`INSERT INTO acme_account (id, account_key_thumbprint, account_jwk, status, created_at)
VALUES (1, ?, ?, 'valid', ?)`,
		thumbprint,
		jwk,
		at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return ACMEAccount{}, false, fmt.Errorf("create ACME account: %w", err)
	}
	account, err := s.ACMEAccount()
	return account, true, err
}

func (s *Store) CreateACMEOrder(o ACMEOrder) error {
	if o.ID == "" || o.ClientIdentifier == "" || o.ChallengeToken == "" || o.ChallengeTokenHash == "" {
		return errors.New("incomplete ACME order")
	}
	if o.AccountID == 0 {
		o.AccountID = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO acme_order (
	id, account_id, client_identifier, status, challenge_token, challenge_token_hash,
	challenge_status, attested_spki_sha256, csr_sha256, certificate_serial,
	certificate_pem, expires_at, created_at, finalized_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID,
		o.AccountID,
		o.ClientIdentifier,
		o.Status,
		o.ChallengeToken,
		o.ChallengeTokenHash,
		o.ChallengeStatus,
		o.AttestedSPKISHA256,
		o.CSRSHA256,
		o.CertificateSerial,
		o.CertificatePEM,
		o.ExpiresAt.UTC().Format(time.RFC3339Nano),
		o.CreatedAt.UTC().Format(time.RFC3339Nano),
		formatOptionalTime(o.FinalizedAt),
	)
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return ErrACMEConflict
		}
		return fmt.Errorf("create ACME order: %w", err)
	}
	return nil
}

func (s *Store) ACMEOrder(id string) (ACMEOrder, error) {
	var o ACMEOrder
	var expires, created string
	var finalized sql.NullString
	err := s.db.QueryRow(
		`SELECT id, account_id, client_identifier, status, challenge_token,
       challenge_token_hash, challenge_status, attested_spki_sha256,
       csr_sha256, certificate_serial, certificate_pem, expires_at,
       created_at, finalized_at
FROM acme_order WHERE id = ?`,
		id,
	).Scan(
		&o.ID,
		&o.AccountID,
		&o.ClientIdentifier,
		&o.Status,
		&o.ChallengeToken,
		&o.ChallengeTokenHash,
		&o.ChallengeStatus,
		&o.AttestedSPKISHA256,
		&o.CSRSHA256,
		&o.CertificateSerial,
		&o.CertificatePEM,
		&expires,
		&created,
		&finalized,
	)
	if err != nil {
		if errors.Is(err, sqlErrNoRows()) {
			return ACMEOrder{}, ErrACMENotFound
		}
		return ACMEOrder{}, fmt.Errorf("read ACME order: %w", err)
	}
	var parseErr error
	if o.ExpiresAt, parseErr = time.Parse(time.RFC3339Nano, expires); parseErr != nil {
		return ACMEOrder{}, fmt.Errorf("parse ACME order expiry: %w", parseErr)
	}
	if o.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, created); parseErr != nil {
		return ACMEOrder{}, fmt.Errorf("parse ACME order created time: %w", parseErr)
	}
	if finalized.Valid {
		t, err := time.Parse(time.RFC3339Nano, finalized.String)
		if err != nil {
			return ACMEOrder{}, fmt.Errorf("parse ACME order finalized time: %w", err)
		}
		o.FinalizedAt = &t
	}
	return o, nil
}


func (s *Store) ACMEOrderByClientIdentifier(clientIdentifier string) (ACMEOrder, error) {
	if clientIdentifier == "" {
		return ACMEOrder{}, ErrACMENotFound
	}
	var id string
	err := s.db.QueryRow(
		`SELECT id FROM acme_order WHERE client_identifier = ?`,
		clientIdentifier,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ACMEOrder{}, ErrACMENotFound
		}
		return ACMEOrder{}, fmt.Errorf("read ACME order by client identifier: %w", err)
	}
	return s.ACMEOrder(id)
}

func (s *Store) MarkACMEChallengeValid(orderID, attestedSPKIHash string) error {
	if attestedSPKIHash == "" {
		return errors.New("attested public key hash is required")
	}
	result, err := s.db.Exec(
		`UPDATE acme_order
SET challenge_status = 'valid', status = 'ready', attested_spki_sha256 = ?
WHERE id = ? AND challenge_status = 'pending' AND status = 'pending'`,
		attestedSPKIHash,
		orderID,
	)
	if err != nil {
		return fmt.Errorf("mark ACME challenge valid: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark ACME challenge rows affected: %w", err)
	}
	if rows != 1 {
		return ErrACMEConflict
	}
	return nil
}

func (s *Store) FinalizeACMEOrder(orderID, csrSHA256, certificateSerial string, certificatePEM []byte, identityFingerprint string, at time.Time) error {
	if csrSHA256 == "" || certificateSerial == "" || len(certificatePEM) == 0 || strings.TrimSpace(identityFingerprint) == "" {
		return errors.New("complete issuance metadata is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin ACME finalization: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.Exec(
		`UPDATE acme_order
SET status = 'valid', csr_sha256 = ?, certificate_serial = ?,
    certificate_pem = ?, finalized_at = ?
WHERE id = ? AND status = 'ready' AND challenge_status = 'valid'`,
		csrSHA256,
		certificateSerial,
		certificatePEM,
		at.UTC().Format(time.RFC3339Nano),
		orderID,
	)
	if err != nil {
		return fmt.Errorf("finalize ACME order: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("finalize ACME order rows affected: %w", err)
	}
	if rows != 1 {
		return ErrACMEConflict
	}
	result, err = tx.Exec(
		`UPDATE installation
SET identity_enrolled_at = ?, identity_cert_fingerprint = ?
WHERE id = 1`,
		at.UTC().Format(time.RFC3339Nano),
		identityFingerprint,
	)
	if err != nil {
		return fmt.Errorf("record identity enrollment: %w", err)
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("identity enrollment rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("installation is not initialized")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ACME finalization: %w", err)
	}
	return nil
}



func (s *Store) ResetEnrollment(profileToken, stage2Token, clientIdentifier string) (Installation, error) {
	if profileToken == "" || stage2Token == "" || clientIdentifier == "" {
		return Installation{}, errors.New("all replacement enrollment credentials are required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Installation{}, fmt.Errorf("begin enrollment reset: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
UPDATE installation
SET profile_token = ?,
    stage2_token = ?,
    client_identifier = ?,
    doh_seen_at = NULL,
    proxy_seen_at = NULL,
    stage2_delivered_at = NULL,
    stage2_delivery_count = 0,
    identity_enrolled_at = NULL,
    identity_cert_fingerprint = ''
WHERE id = 1`, profileToken, stage2Token, clientIdentifier)
	if err != nil {
		return Installation{}, fmt.Errorf("reset installation enrollment state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Installation{}, fmt.Errorf("reset installation rows affected: %w", err)
	}
	if rows != 1 {
		return Installation{}, errors.New("installation is not initialized")
	}
	for _, table := range []string{"acme_nonce", "acme_order", "acme_account"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return Installation{}, fmt.Errorf("clear %s during enrollment reset: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Installation{}, fmt.Errorf("commit enrollment reset: %w", err)
	}
	return s.Installation()
}

func formatOptionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func isSQLiteUniqueConstraint(err error) bool {
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unique constraint") || strings.Contains(lower, "constraint failed")
}

// sqlErrNoRows is a tiny indirection so this file keeps all ACME storage
// behavior together while using the standard database sentinel.
func sqlErrNoRows() error { return sql.ErrNoRows }
