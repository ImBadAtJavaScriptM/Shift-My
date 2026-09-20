package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

type jsonWebKey struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type protectedHeader struct {
	Alg   string      `json:"alg"`
	Nonce string      `json:"nonce"`
	URL   string      `json:"url"`
	JWK   *jsonWebKey `json:"jwk,omitempty"`
	KID   string      `json:"kid,omitempty"`
}

type signedRequest struct {
	Protected string `json:"protected"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`

	Header       protectedHeader `json:"-"`
	PayloadBytes []byte          `json:"-"`
	sigBytes     []byte
}

func parseSignedRequest(body []byte) (*signedRequest, error) {
	var req signedRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode JWS: %w", err)
	}
	if req.Protected == "" || req.Signature == "" {
		return nil, errors.New("JWS protected header and signature are required")
	}
	protected, err := base64.RawURLEncoding.DecodeString(req.Protected)
	if err != nil {
		return nil, fmt.Errorf("decode protected header: %w", err)
	}
	if err := json.Unmarshal(protected, &req.Header); err != nil {
		return nil, fmt.Errorf("parse protected header: %w", err)
	}
	if req.Header.Alg != "ES256" && req.Header.Alg != "ES384" {
		return nil, fmt.Errorf("unsupported JWS algorithm %q", req.Header.Alg)
	}
	payload, err := base64.RawURLEncoding.DecodeString(req.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode JWS payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(req.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode JWS signature: %w", err)
	}
	req.PayloadBytes = payload
	req.sigBytes = sig
	return &req, nil
}

func (r *signedRequest) verify(key *ecdsa.PublicKey) error {
	if key == nil {
		return errors.New("missing account public key")
	}
	var digest []byte
	var width int
	switch r.Header.Alg {
	case "ES256":
		if key.Curve != elliptic.P256() {
			return errors.New("ES256 requires P-256 account key")
		}
		sum := sha256.Sum256([]byte(r.Protected + "." + r.Payload))
		digest = sum[:]
		width = 32
	case "ES384":
		if key.Curve != elliptic.P384() {
			return errors.New("ES384 requires P-384 account key")
		}
		sum := sha512.Sum384([]byte(r.Protected + "." + r.Payload))
		digest = sum[:]
		width = 48
	default:
		return errors.New("unsupported signature algorithm")
	}
	if len(r.sigBytes) != width*2 {
		return fmt.Errorf("invalid ECDSA signature length %d", len(r.sigBytes))
	}
	rr := new(big.Int).SetBytes(r.sigBytes[:width])
	ss := new(big.Int).SetBytes(r.sigBytes[width:])
	if !ecdsa.Verify(key, digest, rr, ss) {
		return errors.New("JWS signature does not verify")
	}
	return nil
}

func (j jsonWebKey) publicKey() (*ecdsa.PublicKey, error) {
	if j.Kty != "EC" {
		return nil, fmt.Errorf("unsupported JWK kty %q", j.Kty)
	}
	var curve elliptic.Curve
	var width int
	switch j.Crv {
	case "P-256":
		curve = elliptic.P256()
		width = 32
	case "P-384":
		curve = elliptic.P384()
		width = 48
	default:
		return nil, fmt.Errorf("unsupported JWK curve %q", j.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil || len(xBytes) != width {
		return nil, errors.New("invalid JWK x coordinate")
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil || len(yBytes) != width {
		return nil, errors.New("invalid JWK y coordinate")
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	if !curve.IsOnCurve(x, y) {
		return nil, errors.New("JWK point is not on curve")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

func (j jsonWebKey) thumbprint() (string, error) {
	if _, err := j.publicKey(); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(struct {
		Crv string `json:"crv"`
		Kty string `json:"kty"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}{Crv: j.Crv, Kty: j.Kty, X: j.X, Y: j.Y})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

func (j jsonWebKey) marshal() (string, error) {
	data, err := json.Marshal(j)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseStoredJWK(raw string) (jsonWebKey, *ecdsa.PublicKey, error) {
	var jwk jsonWebKey
	if err := json.Unmarshal([]byte(raw), &jwk); err != nil {
		return jsonWebKey{}, nil, fmt.Errorf("parse stored account JWK: %w", err)
	}
	key, err := jwk.publicKey()
	if err != nil {
		return jsonWebKey{}, nil, err
	}
	return jwk, key, nil
}
