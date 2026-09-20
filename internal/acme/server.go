package acme

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

const (
	contentTypeJOSE     = "application/jose+json"
	contentTypeJSON     = "application/json"
	contentTypeProblem  = "application/problem+json"
	contentTypePEMChain = "application/pem-certificate-chain"
	maxACMEBody         = 256 << 10
	nonceTTL            = time.Hour
	orderTTL            = 24 * time.Hour
	identityLifetime    = 7 * 24 * time.Hour
)

type Server struct {
	baseURL    string
	store      *storage.Store
	identityCA *pki.Authority
	attest     *attestationVerifier
	now        func() time.Time
}

func New(publicHost string, store *storage.Store, identityCA *pki.Authority) (*Server, error) {
	host := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(publicHost), "."))
	if host == "" || strings.ContainsAny(host, "/: ") {
		return nil, errors.New("valid ACME public host is required")
	}
	if store == nil || identityCA == nil {
		return nil, errors.New("ACME store and identity authority are required")
	}
	attest, err := newAttestationVerifier()
	if err != nil {
		return nil, err
	}
	return &Server{
		baseURL:    "https://" + host + "/acme/device",
		store:      store,
		identityCA: identityCA,
		attest:     attest,
		now:        time.Now,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /acme/device/directory", s.plain(s.directory))
	mux.HandleFunc("HEAD /acme/device/new-nonce", s.plain(s.newNonce))
	mux.HandleFunc("GET /acme/device/new-nonce", s.plain(s.newNonce))
	mux.HandleFunc("POST /acme/device/new-account", s.signed(true, s.newAccount))
	mux.HandleFunc("POST /acme/device/new-order", s.signed(false, s.newOrder))
	mux.HandleFunc("POST /acme/device/account/1", s.signed(false, s.account))
	mux.HandleFunc("POST /acme/device/account/1/orders", s.signed(false, s.accountOrders))
	mux.HandleFunc("POST /acme/device/order/{id}", s.signed(false, s.order))
	mux.HandleFunc("POST /acme/device/order/{id}/finalize", s.signed(false, s.finalize))
	mux.HandleFunc("POST /acme/device/authz/{id}", s.signed(false, s.authorization))
	mux.HandleFunc("POST /acme/device/challenge/{id}", s.signed(false, s.challenge))
	mux.HandleFunc("POST /acme/device/cert/{id}", s.signed(false, s.certificate))
	mux.HandleFunc("/acme/device/", s.plain(func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, http.StatusBadRequest, "malformed", "no such ACME endpoint")
	}))
	return mux
}

func (s *Server) directory(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"newNonce":   s.baseURL + "/new-nonce",
		"newAccount": s.baseURL + "/new-account",
		"newOrder":   s.baseURL + "/new-order",
		"meta": map[string]any{
			"website": strings.TrimSuffix(s.baseURL, "/acme/device"),
		},
	})
}

func (s *Server) newNonce(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type accountRequest struct {
	Contact            []string `json:"contact,omitempty"`
	OnlyReturnExisting bool     `json:"onlyReturnExisting,omitempty"`
}

func (s *Server) newAccount(w http.ResponseWriter, r *http.Request, req *signedRequest, _ storage.ACMEAccount) {
	var payload accountRequest
	if len(req.PayloadBytes) > 0 {
		if err := json.Unmarshal(req.PayloadBytes, &payload); err != nil {
			s.fail(w, http.StatusBadRequest, "malformed", "invalid new-account payload")
			return
		}
	}
	if req.Header.JWK == nil {
		s.fail(w, http.StatusBadRequest, "malformed", "new-account request requires jwk")
		return
	}
	thumbprint, err := req.Header.JWK.thumbprint()
	if err != nil {
		s.fail(w, http.StatusBadRequest, "badPublicKey", "unsupported account key")
		return
	}
	jwkJSON, err := req.Header.JWK.marshal()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not store account key")
		return
	}
	if payload.OnlyReturnExisting {
		account, err := s.store.ACMEAccount()
		if err != nil || account.Thumbprint != thumbprint {
			s.fail(w, http.StatusBadRequest, "accountDoesNotExist", "account does not exist")
			return
		}
		w.Header().Set("Location", s.baseURL+"/account/1")
		s.writeJSON(w, http.StatusOK, s.accountBody())
		return
	}
	_, created, err := s.store.EnsureACMEAccount(thumbprint, jwkJSON, s.now())
	if errors.Is(err, storage.ErrACMEConflict) {
		s.fail(w, http.StatusUnauthorized, "unauthorized", "another account is already enrolled")
		return
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not create account")
		return
	}
	w.Header().Set("Location", s.baseURL+"/account/1")
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	s.writeJSON(w, status, s.accountBody())
}

func (s *Server) accountBody() map[string]any {
	return map[string]any{
		"status": "valid",
		"orders": s.baseURL + "/account/1/orders",
	}
}

func (s *Server) account(w http.ResponseWriter, _ *http.Request, req *signedRequest, _ storage.ACMEAccount) {
	if len(req.PayloadBytes) != 0 {
		s.fail(w, http.StatusBadRequest, "malformed", "account updates are not supported")
		return
	}
	s.writeJSON(w, http.StatusOK, s.accountBody())
}

func (s *Server) accountOrders(w http.ResponseWriter, _ *http.Request, req *signedRequest, _ storage.ACMEAccount) {
	if len(req.PayloadBytes) != 0 {
		s.fail(w, http.StatusBadRequest, "malformed", "orders listing is POST-as-GET only")
		return
	}
	// This single-device implementation does not need order history discovery;
	// the active order URL is already returned by new-order.
	s.writeJSON(w, http.StatusOK, map[string]any{"orders": []string{}})
}

type identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type newOrderRequest struct {
	Identifiers []identifier `json:"identifiers"`
	NotBefore   string       `json:"notBefore,omitempty"`
	NotAfter    string       `json:"notAfter,omitempty"`
}

func (s *Server) newOrder(w http.ResponseWriter, _ *http.Request, req *signedRequest, account storage.ACMEAccount) {
	var payload newOrderRequest
	if err := json.Unmarshal(req.PayloadBytes, &payload); err != nil {
		s.fail(w, http.StatusBadRequest, "malformed", "invalid new-order payload")
		return
	}
	if len(payload.Identifiers) != 1 {
		s.fail(w, http.StatusBadRequest, "malformed", "exactly one identifier is required")
		return
	}
	id := payload.Identifiers[0]
	if id.Type != "permanent-identifier" {
		s.fail(w, http.StatusBadRequest, "unsupportedIdentifier", "only permanent-identifier is supported")
		return
	}
	if payload.NotBefore != "" || payload.NotAfter != "" {
		s.fail(w, http.StatusBadRequest, "malformed", "requested validity windows are not supported")
		return
	}
	inst, err := s.store.Installation()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not read enrollment")
		return
	}
	if id.Value == "" || !constantTimeStringEqual(id.Value, inst.ClientIdentifier) {
		s.fail(w, http.StatusBadRequest, "rejectedIdentifier", "client identifier is not valid for this enrollment")
		return
	}
	if inst.IdentityEnrolledAt != nil {
		s.fail(w, http.StatusBadRequest, "rejectedIdentifier", "this enrollment already has an identity")
		return
	}

	orderID, err := randomURLToken(16)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not create order")
		return
	}
	challengeToken, err := randomURLToken(32)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not create challenge")
		return
	}
	challengeHash := sha256.Sum256([]byte(challengeToken))
	now := s.now().UTC()
	order := storage.ACMEOrder{
		ID:                 orderID,
		AccountID:          account.ID,
		ClientIdentifier:   id.Value,
		Status:             "pending",
		ChallengeToken:     challengeToken,
		ChallengeTokenHash: hex.EncodeToString(challengeHash[:]),
		ChallengeStatus:    "pending",
		ExpiresAt:          now.Add(orderTTL),
		CreatedAt:          now,
	}
	if err := s.store.CreateACMEOrder(order); err != nil {
		if errors.Is(err, storage.ErrACMEConflict) {
			s.fail(w, http.StatusBadRequest, "rejectedIdentifier", "client identifier has already been used")
			return
		}
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not store order")
		return
	}
	w.Header().Set("Location", s.baseURL+"/order/"+order.ID)
	s.writeJSON(w, http.StatusCreated, s.orderBody(order))
}

func (s *Server) order(w http.ResponseWriter, r *http.Request, req *signedRequest, account storage.ACMEAccount) {
	if len(req.PayloadBytes) != 0 {
		s.fail(w, http.StatusBadRequest, "malformed", "order retrieval is POST-as-GET only")
		return
	}
	o, ok := s.loadOwnedOrder(w, r.PathValue("id"), account)
	if !ok {
		return
	}
	s.writeJSON(w, http.StatusOK, s.orderBody(o))
}

func (s *Server) orderBody(o storage.ACMEOrder) map[string]any {
	body := map[string]any{
		"status":      o.Status,
		"expires":     o.ExpiresAt.UTC().Format(time.RFC3339),
		"identifiers": []identifier{{Type: "permanent-identifier", Value: o.ClientIdentifier}},
		"authorizations": []string{
			s.baseURL + "/authz/" + o.ID,
		},
		"finalize": s.baseURL + "/order/" + o.ID + "/finalize",
	}
	if o.Status == "valid" && len(o.CertificatePEM) > 0 {
		body["certificate"] = s.baseURL + "/cert/" + o.ID
	}
	return body
}

func (s *Server) authorization(w http.ResponseWriter, r *http.Request, req *signedRequest, account storage.ACMEAccount) {
	if len(req.PayloadBytes) != 0 {
		s.fail(w, http.StatusBadRequest, "malformed", "authorization retrieval is POST-as-GET only")
		return
	}
	o, ok := s.loadOwnedOrder(w, r.PathValue("id"), account)
	if !ok {
		return
	}
	status := "pending"
	if o.ChallengeStatus == "valid" {
		status = "valid"
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":     status,
		"expires":    o.ExpiresAt.UTC().Format(time.RFC3339),
		"identifier": identifier{Type: "permanent-identifier", Value: o.ClientIdentifier},
		"challenges": []any{
			s.challengeBody(o),
		},
	})
}

func (s *Server) challengeBody(o storage.ACMEOrder) map[string]any {
	return map[string]any{
		"type":   "device-attest-01",
		"url":    s.baseURL + "/challenge/" + o.ID,
		"status": o.ChallengeStatus,
		"token":  o.ChallengeToken,
	}
}

type challengeRequest struct {
	AttObj string `json:"attObj"`
}

func (s *Server) challenge(w http.ResponseWriter, r *http.Request, req *signedRequest, account storage.ACMEAccount) {
	o, ok := s.loadOwnedOrder(w, r.PathValue("id"), account)
	if !ok {
		return
	}
	if len(req.PayloadBytes) == 0 || o.ChallengeStatus != "pending" {
		s.writeJSON(w, http.StatusOK, s.challengeBody(o))
		return
	}
	if s.now().After(o.ExpiresAt) {
		s.fail(w, http.StatusBadRequest, "malformed", "authorization has expired")
		return
	}
	var payload challengeRequest
	if err := json.Unmarshal(req.PayloadBytes, &payload); err != nil || payload.AttObj == "" {
		s.fail(w, http.StatusBadRequest, "malformed", "challenge response requires attObj")
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload.AttObj)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "malformed", "attObj must be base64url")
		return
	}
	spkiHash, err := s.attest.verify(raw, o.ChallengeToken)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "badAttestationStatement", "managed-device attestation did not verify")
		return
	}
	if err := s.store.MarkACMEChallengeValid(o.ID, spkiHash); err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not record attestation")
		return
	}
	o, _ = s.store.ACMEOrder(o.ID)
	s.writeJSON(w, http.StatusOK, s.challengeBody(o))
}

type finalizeRequest struct {
	CSR string `json:"csr"`
}

func (s *Server) finalize(w http.ResponseWriter, r *http.Request, req *signedRequest, account storage.ACMEAccount) {
	o, ok := s.loadOwnedOrder(w, r.PathValue("id"), account)
	if !ok {
		return
	}
	if o.Status == "valid" {
		s.writeJSON(w, http.StatusOK, s.orderBody(o))
		return
	}
	if o.Status != "ready" || o.ChallengeStatus != "valid" {
		s.fail(w, http.StatusForbidden, "orderNotReady", "attestation challenge is not valid")
		return
	}
	var payload finalizeRequest
	if err := json.Unmarshal(req.PayloadBytes, &payload); err != nil || payload.CSR == "" {
		s.fail(w, http.StatusBadRequest, "badCSR", "finalize requires a CSR")
		return
	}
	csrDER, err := base64.RawURLEncoding.DecodeString(payload.CSR)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "badCSR", "CSR must be base64url")
		return
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil || csr.CheckSignature() != nil {
		s.fail(w, http.StatusBadRequest, "badCSR", "CSR signature is invalid")
		return
	}
	csrSPKI, err := spkiSHA256(csr.PublicKey)
	if err != nil || !constantTimeStringEqual(csrSPKI, o.AttestedSPKISHA256) {
		s.fail(w, http.StatusBadRequest, "badCSR", "CSR key does not match attested hardware-bound key")
		return
	}
	issued, err := s.identityCA.SignClientCSR(csrDER, o.ClientIdentifier, identityLifetime)
	if err != nil {
		s.fail(w, http.StatusBadRequest, "badCSR", "CSR does not meet identity certificate policy")
		return
	}
	csrHash := sha256.Sum256(csrDER)
	if err := s.store.FinalizeACMEOrder(
		o.ID,
		hex.EncodeToString(csrHash[:]),
		issued.Leaf.SerialNumber.Text(16),
		issued.ChainPEM,
		s.now(),
	); err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not persist issued certificate")
		return
	}
	if err := s.store.MarkIdentityEnrolled(s.now(), issued.Fingerprint); err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not record identity enrollment")
		return
	}
	o, _ = s.store.ACMEOrder(o.ID)
	s.writeJSON(w, http.StatusOK, s.orderBody(o))
}

func (s *Server) certificate(w http.ResponseWriter, r *http.Request, req *signedRequest, account storage.ACMEAccount) {
	if len(req.PayloadBytes) != 0 {
		s.fail(w, http.StatusBadRequest, "malformed", "certificate retrieval is POST-as-GET only")
		return
	}
	o, ok := s.loadOwnedOrder(w, r.PathValue("id"), account)
	if !ok {
		return
	}
	if o.Status != "valid" || len(o.CertificatePEM) == 0 {
		s.fail(w, http.StatusForbidden, "orderNotReady", "certificate is not ready")
		return
	}
	w.Header().Set("Content-Type", contentTypePEMChain)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(o.CertificatePEM)
}

type signedEndpoint func(http.ResponseWriter, *http.Request, *signedRequest, storage.ACMEAccount)

func (s *Server) signed(newAccount bool, next signedEndpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.addNonce(w)
		w.Header().Add("Link", "<"+s.baseURL+"/directory>;rel=\"index\"")
		if !strings.HasPrefix(r.Header.Get("Content-Type"), contentTypeJOSE) {
			s.fail(w, http.StatusBadRequest, "malformed", "application/jose+json is required")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxACMEBody+1))
		if err != nil || len(body) > maxACMEBody {
			s.fail(w, http.StatusBadRequest, "malformed", "ACME request body is invalid")
			return
		}
		jws, err := parseSignedRequest(body)
		if err != nil {
			s.fail(w, http.StatusBadRequest, "malformed", "ACME request is not a valid JWS")
			return
		}
		if jws.Header.URL != s.baseURL+r.URL.Path[strings.Index(r.URL.Path, "/acme/device")+len("/acme/device"):] {
			s.fail(w, http.StatusBadRequest, "malformed", "JWS url does not match request URL")
			return
		}
		ok, err := s.store.ConsumeACMENonce(jws.Header.Nonce, s.now())
		if err != nil {
			s.fail(w, http.StatusInternalServerError, "serverInternal", "could not validate nonce")
			return
		}
		if !ok {
			s.fail(w, http.StatusBadRequest, "badNonce", "nonce is missing, expired, or already used")
			return
		}

		if newAccount {
			if jws.Header.JWK == nil || jws.Header.KID != "" {
				s.fail(w, http.StatusBadRequest, "malformed", "new-account must use jwk and not kid")
				return
			}
			key, err := jws.Header.JWK.publicKey()
			if err != nil || jws.verify(key) != nil {
				s.fail(w, http.StatusUnauthorized, "unauthorized", "new-account signature does not verify")
				return
			}
			next(w, r, jws, storage.ACMEAccount{})
			return
		}

		if jws.Header.JWK != nil || jws.Header.KID != s.baseURL+"/account/1" {
			s.fail(w, http.StatusUnauthorized, "accountDoesNotExist", "request must use the enrolled account")
			return
		}
		account, err := s.store.ACMEAccount()
		if err != nil || account.Status != "valid" {
			s.fail(w, http.StatusUnauthorized, "accountDoesNotExist", "account does not exist")
			return
		}
		_, key, err := parseStoredJWK(account.JWK)
		if err != nil || jws.verify(key) != nil {
			s.fail(w, http.StatusUnauthorized, "unauthorized", "account signature does not verify")
			return
		}
		next(w, r, jws, account)
	}
}

func (s *Server) plain(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.addNonce(w)
		w.Header().Add("Link", "<"+s.baseURL+"/directory>;rel=\"index\"")
		next(w, r)
	}
}

func (s *Server) addNonce(w http.ResponseWriter) {
	nonce, err := randomURLToken(24)
	if err != nil {
		return
	}
	if err := s.store.PutACMENonce(nonce, s.now().Add(nonceTTL)); err != nil {
		return
	}
	w.Header().Set("Replay-Nonce", nonce)
	w.Header().Set("Cache-Control", "no-store")
}

func (s *Server) loadOwnedOrder(w http.ResponseWriter, id string, account storage.ACMEAccount) (storage.ACMEOrder, bool) {
	o, err := s.store.ACMEOrder(id)
	if errors.Is(err, storage.ErrACMENotFound) {
		s.fail(w, http.StatusBadRequest, "malformed", "no such order")
		return storage.ACMEOrder{}, false
	}
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not read order")
		return storage.ACMEOrder{}, false
	}
	if o.AccountID != account.ID {
		s.fail(w, http.StatusUnauthorized, "unauthorized", "order belongs to another account")
		return storage.ACMEOrder{}, false
	}
	return o, true
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "serverInternal", "could not encode response")
		return
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func (s *Server) fail(w http.ResponseWriter, status int, problemType, detail string) {
	w.Header().Set("Content-Type", contentTypeProblem)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "urn:ietf:params:acme:error:" + problemType,
		"detail": detail,
		"status": status,
	})
}

func randomURLToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func constantTimeStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
