package acme

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	"github.com/fxamacker/cbor/v2"
)

func TestAttestedACMEFlowIssuesClientOnlyIdentity(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureInstallation("legacy-token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureEnrollmentCredentials("doh-token", "stage2-token", "client-one"); err != nil {
		t.Fatal(err)
	}

	identityCA := testAuthority(t, "Identity CA")
	srv, err := New("lab.example.test", store, identityCA)
	if err != nil {
		t.Fatal(err)
	}
	attRoot, attRootKey := testRoot(t, "Fake Apple Enterprise Attestation Root")
	srv.attest.roots = []*x509.Certificate{attRoot}

	h := srv.Handler()
	accountKey := mustECDSAKey(t)
	nonce := getNonce(t, h)

	newAccountPayload := []byte(`{"termsOfServiceAgreed":true}`)
	rr := signedPOST(t, h, "/acme/device/new-account", nonce, newAccountPayload, accountKey, true)
	if rr.Code != http.StatusCreated {
		t.Fatalf("new account status=%d body=%s", rr.Code, rr.Body.String())
	}
	nonce = rr.Header().Get("Replay-Nonce")

	orderPayload := []byte(`{"identifiers":[{"type":"permanent-identifier","value":"client-one"}]}`)
	rr = signedPOST(t, h, "/acme/device/new-order", nonce, orderPayload, accountKey, false)
	if rr.Code != http.StatusCreated {
		t.Fatalf("new order status=%d body=%s", rr.Code, rr.Body.String())
	}
	nonce = rr.Header().Get("Replay-Nonce")
	var order map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	orderURL := rr.Header().Get("Location")
	orderID := filepath.Base(orderURL)
	authzURL := order["authorizations"].([]any)[0].(string)

	rr = signedPOST(t, h, pathFromURL(authzURL), nonce, nil, accountKey, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("authz status=%d body=%s", rr.Code, rr.Body.String())
	}
	nonce = rr.Header().Get("Replay-Nonce")
	var authz map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &authz); err != nil {
		t.Fatal(err)
	}
	challenge := authz["challenges"].([]any)[0].(map[string]any)
	token := challenge["token"].(string)
	challengeURL := challenge["url"].(string)

	deviceKey := mustECDSAKey(t)
	attObj := testAppleAttestationObject(t, attRoot, attRootKey, &deviceKey.PublicKey, token)
	challengePayload, _ := json.Marshal(map[string]string{
		"attObj": base64.RawURLEncoding.EncodeToString(attObj),
	})
	rr = signedPOST(t, h, pathFromURL(challengeURL), nonce, challengePayload, accountKey, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("challenge status=%d body=%s", rr.Code, rr.Body.String())
	}
	nonce = rr.Header().Get("Replay-Nonce")

	csrDER := testCSR(t, deviceKey)
	finalizePayload, _ := json.Marshal(map[string]string{
		"csr": base64.RawURLEncoding.EncodeToString(csrDER),
	})
	rr = signedPOST(t, h, "/acme/device/order/"+orderID+"/finalize", nonce, finalizePayload, accountKey, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("finalize status=%d body=%s", rr.Code, rr.Body.String())
	}
	nonce = rr.Header().Get("Replay-Nonce")
	var finalized map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &finalized); err != nil {
		t.Fatal(err)
	}
	if finalized["status"] != "valid" {
		t.Fatalf("order status=%v", finalized["status"])
	}

	rr = signedPOST(t, h, "/acme/device/cert/"+orderID, nonce, nil, accountKey, false)
	if rr.Code != http.StatusOK {
		t.Fatalf("certificate status=%d body=%s", rr.Code, rr.Body.String())
	}
	block, _ := pem.Decode(rr.Body.Bytes())
	if block == nil {
		t.Fatal("issued PEM certificate missing")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatalf("ExtKeyUsage=%v", leaf.ExtKeyUsage)
	}
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			t.Fatal("identity certificate must not permit server authentication")
		}
	}
	if leaf.Subject.CommonName != "client-one" {
		t.Fatalf("CN=%q", leaf.Subject.CommonName)
	}

	inst, err := store.Installation()
	if err != nil {
		t.Fatal(err)
	}
	if inst.IdentityEnrolledAt == nil || inst.IdentityCertFingerprint == "" {
		t.Fatalf("identity state=%+v", inst)
	}
}

func TestACMENonceIsSingleUse(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureInstallation("token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureEnrollmentCredentials("doh", "stage2", "client"); err != nil {
		t.Fatal(err)
	}
	srv, err := New("lab.example.test", store, testAuthority(t, "Identity CA"))
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	key := mustECDSAKey(t)
	nonce := getNonce(t, h)
	payload := []byte(`{}`)

	first := signedPOST(t, h, "/acme/device/new-account", nonce, payload, key, true)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	second := signedPOST(t, h, "/acme/device/new-account", nonce, payload, key, true)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("replay status=%d body=%s", second.Code, second.Body.String())
	}
	if !bytes.Contains(second.Body.Bytes(), []byte("badNonce")) {
		t.Fatalf("replay body=%s", second.Body.String())
	}
}

func TestACMERejectsWrongClientIdentifier(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureInstallation("token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureEnrollmentCredentials("doh", "stage2", "expected-client"); err != nil {
		t.Fatal(err)
	}
	srv, err := New("lab.example.test", store, testAuthority(t, "Identity CA"))
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	key := mustECDSAKey(t)
	nonce := getNonce(t, h)
	rr := signedPOST(t, h, "/acme/device/new-account", nonce, []byte(`{}`), key, true)
	nonce = rr.Header().Get("Replay-Nonce")
	payload := []byte(`{"identifiers":[{"type":"permanent-identifier","value":"wrong-client"}]}`)
	rr = signedPOST(t, h, "/acme/device/new-order", nonce, payload, key, false)
	if rr.Code != http.StatusBadRequest || !bytes.Contains(rr.Body.Bytes(), []byte("rejectedIdentifier")) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestACMERejectsInvalidAttestation(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureInstallation("token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureEnrollmentCredentials("doh", "stage2", "client"); err != nil {
		t.Fatal(err)
	}
	srv, err := New("lab.example.test", store, testAuthority(t, "Identity CA"))
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	key := mustECDSAKey(t)
	nonce := getNonce(t, h)
	rr := signedPOST(t, h, "/acme/device/new-account", nonce, []byte(`{}`), key, true)
	nonce = rr.Header().Get("Replay-Nonce")
	rr = signedPOST(t, h, "/acme/device/new-order", nonce, []byte(`{"identifiers":[{"type":"permanent-identifier","value":"client"}]}`), key, false)
	nonce = rr.Header().Get("Replay-Nonce")
	orderID := filepath.Base(rr.Header().Get("Location"))
	payload, _ := json.Marshal(map[string]string{"attObj": base64.RawURLEncoding.EncodeToString([]byte("not-cbor"))})
	rr = signedPOST(t, h, "/acme/device/challenge/"+orderID, nonce, payload, key, false)
	if rr.Code != http.StatusBadRequest || !bytes.Contains(rr.Body.Bytes(), []byte("badAttestationStatement")) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func getNonce(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodHead, "https://lab.example.test/acme/device/new-nonce", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("nonce status=%d", rr.Code)
	}
	nonce := rr.Header().Get("Replay-Nonce")
	if nonce == "" {
		t.Fatal("Replay-Nonce missing")
	}
	return nonce
}

func signedPOST(t *testing.T, h http.Handler, path, nonce string, payload []byte, key *ecdsa.PrivateKey, newAccount bool) *httptest.ResponseRecorder {
	t.Helper()
	jwk := jwkForKey(t, &key.PublicKey)
	protected := map[string]any{
		"alg":   "ES256",
		"nonce": nonce,
		"url":   "https://lab.example.test" + path,
	}
	if newAccount {
		protected["jwk"] = jwk
	} else {
		protected["kid"] = "https://lab.example.test/acme/device/account/1"
	}
	protectedJSON, _ := json.Marshal(protected)
	protected64 := base64.RawURLEncoding.EncodeToString(protectedJSON)
	payload64 := ""
	if payload != nil {
		payload64 = base64.RawURLEncoding.EncodeToString(payload)
	}
	sum := sha256.Sum256([]byte(protected64 + "." + payload64))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(paddedInt(r, 32), paddedInt(s, 32)...)
	body, _ := json.Marshal(map[string]string{
		"protected": protected64,
		"payload":   payload64,
		"signature": base64.RawURLEncoding.EncodeToString(sig),
	})
	req := httptest.NewRequest(http.MethodPost, "https://lab.example.test"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentTypeJOSE)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func jwkForKey(t *testing.T, key *ecdsa.PublicKey) jsonWebKey {
	t.Helper()
	return jsonWebKey{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(paddedInt(key.X, 32)),
		Y:   base64.RawURLEncoding.EncodeToString(paddedInt(key.Y, 32)),
	}
}

func paddedInt(n *big.Int, width int) []byte {
	out := make([]byte, width)
	b := n.Bytes()
	copy(out[width-len(b):], b)
	return out
}

func mustECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testAuthority(t *testing.T, name string) *pki.Authority {
	t.Helper()
	certPEM, keyPEM, err := pki.GenerateRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath := dir + "/ca.pem"
	keyPath := dir + "/ca-key.pem"
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := pki.Load(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func testRoot(t *testing.T, name string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key := mustECDSAKey(t)
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func testAppleAttestationObject(t *testing.T, root *x509.Certificate, rootKey *ecdsa.PrivateKey, deviceKey *ecdsa.PublicKey, token string) []byte {
	t.Helper()
	now := time.Now().UTC()
	freshness := sha256.Sum256([]byte(token))
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Fake Apple Attestation Leaf"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{
			{Id: oidAppleFreshness, Value: freshness[:]},
		},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, root, deviceKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{
		"fmt": "apple",
		"attStmt": map[string]any{
			"x5c": [][]byte{der},
		},
	}
	raw, err := cbor.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func testCSR(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "requested-but-overridden"},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func pathFromURL(raw string) string {
	const origin = "https://lab.example.test"
	return strings.TrimPrefix(raw, origin)
}


func TestFinalizeRejectsCSRForDifferentKeyThanAttestation(t *testing.T) {
	store, err := storage.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureInstallation("token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureEnrollmentCredentials("doh", "stage2", "client"); err != nil {
		t.Fatal(err)
	}
	srv, err := New("lab.example.test", store, testAuthority(t, "Identity CA"))
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	accountKey := mustECDSAKey(t)
	nonce := getNonce(t, h)
	rr := signedPOST(t, h, "/acme/device/new-account", nonce, []byte(`{}`), accountKey, true)
	if rr.Code != http.StatusCreated {
		t.Fatalf("account status=%d body=%s", rr.Code, rr.Body.String())
	}

	attestedKey := mustECDSAKey(t)
	attestedHash, err := spkiSHA256(&attestedKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	order := storage.ACMEOrder{
		ID:                 "mismatch-order",
		AccountID:          1,
		ClientIdentifier:   "client",
		Status:             "pending",
		ChallengeToken:     "challenge",
		ChallengeTokenHash: "hash",
		ChallengeStatus:    "pending",
		ExpiresAt:          now.Add(time.Hour),
		CreatedAt:          now,
	}
	if err := store.CreateACMEOrder(order); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkACMEChallengeValid(order.ID, attestedHash); err != nil {
		t.Fatal(err)
	}

	nonce = getNonce(t, h)
	differentKey := mustECDSAKey(t)
	csrDER := testCSR(t, differentKey)
	payload, _ := json.Marshal(map[string]string{"csr": base64.RawURLEncoding.EncodeToString(csrDER)})
	rr = signedPOST(t, h, "/acme/device/order/"+order.ID+"/finalize", nonce, payload, accountKey, false)
	if rr.Code != http.StatusBadRequest || !bytes.Contains(rr.Body.Bytes(), []byte("badCSR")) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	got, err := store.ACMEOrder(order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ready" || len(got.CertificatePEM) != 0 {
		t.Fatalf("mismatched CSR changed order: %+v", got)
	}
}
