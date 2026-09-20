package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	acmeserver "github.com/ImBadAtJavaScriptM/Shift-My/internal/acme"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/acme"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/capture"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/config"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/dashboard"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/doh"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/location"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/netpolicy"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/profile"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/proxy"
	publicserver "github.com/ImBadAtJavaScriptM/Shift-My/internal/server"
	"github.com/ImBadAtJavaScriptM/Shift-My/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	profileToken, err := randomToken()
	if err != nil {
		return err
	}
	if err := store.EnsureInstallation(profileToken); err != nil {
		return err
	}
	stage2Token, err := randomToken()
	if err != nil {
		return err
	}
	clientIdentifier, err := randomToken()
	if err != nil {
		return err
	}
	if err := store.EnsureEnrollmentCredentials(profileToken, stage2Token, clientIdentifier); err != nil {
		return err
	}

	policy, err := netpolicy.NewControlled(cfg.PublicHost)
	if err != nil {
		return err
	}
	authority, err := pki.Load(cfg.CACertPath, cfg.CAKeyPath)
	if err != nil {
		return err
	}
	identityAuthority, err := pki.Load(cfg.IdentityCACertPath, cfg.IdentityCAKeyPath)
	if err != nil {
		return err
	}
	recorder, err := capture.New(cfg.CaptureEnabled, cfg.CaptureDir)
	if err != nil {
		return err
	}

	profileCfg := profile.Config{
		DisplayName:  "Shift-My Test",
		PublicHost:   cfg.PublicHost,
		PublicIP:     cfg.PublicIP,
		RootCertDER:  authority.RootDER(),
		MatchDomains: policy.Hosts(),
	}
	dashboardHandler := dashboard.New(store, location.New(store), dashboard.WithProfileConfig(profileCfg))
	dohHandler := doh.New(store, cfg.PublicIP, policy)
	acmeServer, err := acmeserver.New(cfg.PublicHost, store, identityAuthority)
	if err != nil {
		return err
	}
	publicHandler := publicserver.NewPublic(cfg.PublicHost, cfg.AdminPassword, dashboardHandler, dohHandler, acmeServer.Handler())
	labHandler := proxy.NewWithRecorder(authority, store, policy, recorder)

	publicCert, err := tls.LoadX509KeyPair(cfg.PublicCertPath, cfg.PublicKeyPath)
	if err != nil {
		return fmt.Errorf("load public TLS certificate: %w", err)
	}
	publicListener, err := net.Listen("tcp", "127.0.0.1:8443")
	if err != nil {
		return fmt.Errorf("listen public TLS: %w", err)
	}
	defer publicListener.Close()
	labListener, err := net.Listen("tcp", "127.0.0.1:9443")
	if err != nil {
		return fmt.Errorf("listen lab TLS: %w", err)
	}
	defer labListener.Close()

	publicTLS := tls.NewListener(publicListener, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{publicCert},
	})
	labTLS := tls.NewListener(labListener, &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return labHandler.CertificateForHost(hello.ServerName)
		},
	})

	publicHTTP := &http.Server{Handler: publicHandler, ReadHeaderTimeout: 10 * time.Second}
	labHTTP := &http.Server{Handler: labHandler, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 2)
	go func() {
		log.Printf("public dashboard/DoH TLS listening on 127.0.0.1:8443 for %s", cfg.PublicHost)
		errCh <- publicHTTP.Serve(publicTLS)
	}()
	go func() {
		log.Printf("controlled lab TLS listening on 127.0.0.1:9443 for %v", policy.Hosts())
		errCh <- labHTTP.Serve(labTLS)
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
		log.Printf("shutdown requested")
	case serveErr := <-errCh:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve TLS: %w", serveErr)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := publicHTTP.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown public server: %w", err)
	}
	if err := labHTTP.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown lab server: %w", err)
	}
	return nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
