package config

import "testing"

func TestLoadRequiresPublicHostIPv4AndAdminPassword(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "")
	t.Setenv("SHIFT_MY_PUBLIC_IPV4", "")
	t.Setenv("SHIFT_MY_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing host/IP/admin password error")
	}
}

func TestLoadRejectsMissingAdminPassword(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
	t.Setenv("SHIFT_MY_PUBLIC_IPV4", "203.0.113.10")
	t.Setenv("SHIFT_MY_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing admin password error")
	}
}

func TestLoadAcceptsValidIPv4AndAdminPassword(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
	t.Setenv("SHIFT_MY_PUBLIC_IPV4", "203.0.113.10")
	t.Setenv("SHIFT_MY_ADMIN_PASSWORD", "secret-password")
	t.Setenv("SHIFT_MY_DB_PATH", t.TempDir()+"/test.db")
	t.Setenv("SHIFT_MY_CA_CERT", "/tmp/ca.pem")
	t.Setenv("SHIFT_MY_CA_KEY", "/tmp/ca-key.pem")
	t.Setenv("SHIFT_MY_PUBLIC_CERT", "/tmp/fullchain.pem")
	t.Setenv("SHIFT_MY_PUBLIC_KEY", "/tmp/privkey.pem")
	t.Setenv("SHIFT_MY_CAPTURE_ENABLED", "false")
	cfg, err := Load()
	if err != nil { t.Fatal(err) }
	if cfg.PublicHost != "test.example.com" { t.Fatalf("host=%q", cfg.PublicHost) }
	if cfg.AdminPassword != "secret-password" { t.Fatal("admin password was not loaded") }
}
