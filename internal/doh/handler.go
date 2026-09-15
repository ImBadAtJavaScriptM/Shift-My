package doh

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/netpolicy"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
	"github.com/miekg/dns"
)

const maxDNSMessage = 4096

type handler struct {
	store    *storage.Store
	publicIP net.IP
	policy   netpolicy.Policy
}

func New(store *storage.Store, publicIP net.IP, policy netpolicy.Policy) http.Handler {
	return &handler{store: store, publicIP: append(net.IP(nil), publicIP...), policy: policy}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, ok := tokenFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	inst, err := h.store.Installation()
	if err != nil {
		http.Error(w, "read installation", http.StatusInternalServerError)
		return
	}
	if !secureTokenEqual(token, inst.ProfileToken) {
		http.NotFound(w, r)
		return
	}

	wire, status, err := requestWire(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}

	query := new(dns.Msg)
	if err := query.Unpack(wire); err != nil {
		http.Error(w, "invalid DNS message", http.StatusBadRequest)
		return
	}
	if len(query.Question) != 1 {
		http.Error(w, "exactly one DNS question is required", http.StatusBadRequest)
		return
	}

	q := query.Question[0]
	response := new(dns.Msg)
	if !h.policy.AllowedHost(q.Name) {
		response.SetRcode(query, dns.RcodeRefused)
		h.writeDNS(w, response)
		return
	}

	response.SetReply(query)
	allowedType := false
	switch q.Qtype {
	case dns.TypeA:
		allowedType = true
		if ip := h.publicIP.To4(); ip != nil {
			response.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
				A:   append(net.IP(nil), ip...),
			}}
		}
	case dns.TypeAAAA:
		allowedType = true
		if h.publicIP.To4() == nil {
			if ip := h.publicIP.To16(); ip != nil {
				response.Answer = []dns.RR{&dns.AAAA{
					Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 30},
					AAAA: append(net.IP(nil), ip...),
				}}
			}
		}
	default:
		response.SetRcode(query, dns.RcodeRefused)
	}

	if allowedType {
		if err := h.store.MarkDoHSeen(time.Now().UTC()); err != nil {
			http.Error(w, "record DoH activity", http.StatusInternalServerError)
			return
		}
	}
	h.writeDNS(w, response)
}

func tokenFromPath(path string) (string, bool) {
	const prefix = "/dns-query/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(path, prefix)
	if token == "" || strings.Contains(token, "/") {
		return "", false
	}
	return token, true
}

func secureTokenEqual(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func requestWire(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	switch r.Method {
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/dns-message" {
			return nil, http.StatusUnsupportedMediaType, errors.New("application/dns-message required")
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxDNSMessage)
		wire, err := io.ReadAll(r.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return nil, http.StatusRequestEntityTooLarge, errors.New("DNS message too large")
			}
			return nil, http.StatusBadRequest, errors.New("read DNS message")
		}
		if len(wire) == 0 {
			return nil, http.StatusBadRequest, errors.New("empty DNS message")
		}
		return wire, http.StatusOK, nil
	case http.MethodGet:
		encoded := r.URL.Query().Get("dns")
		if encoded == "" {
			return nil, http.StatusBadRequest, errors.New("dns query parameter required")
		}
		wire, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("invalid dns query parameter")
		}
		if len(wire) == 0 || len(wire) > maxDNSMessage {
			if len(wire) > maxDNSMessage {
				return nil, http.StatusRequestEntityTooLarge, errors.New("DNS message too large")
			}
			return nil, http.StatusBadRequest, errors.New("empty DNS message")
		}
		return wire, http.StatusOK, nil
	default:
		return nil, http.StatusMethodNotAllowed, errors.New("method not allowed")
	}
}

func (h *handler) writeDNS(w http.ResponseWriter, msg *dns.Msg) {
	wire, err := msg.Pack()
	if err != nil {
		http.Error(w, "encode DNS response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/dns-message")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(wire)
}
