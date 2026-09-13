package main

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/config"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/dashboard"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/location"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/profile"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	token, err := randomToken()
	if err != nil {
		log.Fatal(err)
	}
	if err := store.EnsureInstallation(token); err != nil {
		log.Fatal(err)
	}

	authority, err := pki.Load(cfg.CACertPath, cfg.CAKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	profileCfg := profile.Config{
		DisplayName:  "Shift-My Test",
		PublicHost:   cfg.PublicHost,
		PublicIPv4:   cfg.PublicIPv4,
		RootCertDER:  authority.RootDER(),
		MatchDomains: profile.LabDomains(cfg.PublicHost),
	}
	handler := dashboard.New(store, location.New(store), dashboard.WithProfileConfig(profileCfg))
	const addr = "127.0.0.1:8080"
	log.Printf("Shift-My test dashboard listening on http://%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
