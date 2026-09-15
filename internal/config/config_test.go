package config

import "testing"

func TestLoadRequiresPublicHostIPAndAdminPassword(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "")
	t.Setenv("SHIFT_MY_PUBLIC_IP", "")
	t.Setenv("SHIFT_MY_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing host/IP/admin password error")
	}
}

func TestLoadRejectsMissingAdminPassword(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
	t.Setenv("SHIFT_MY_PUBLIC_IP", "2001:db8::10")
	t.Setenv("SHIFT_MY_ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing admin password error")
	}
}

func TestLoadAcceptsValidIPv6AndAdminPassword(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
	t.Setenv("SHIFT_MY_PUBLIC_IP", "2001:db8::10")
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
	if got := cfg.PublicIP.String(); got != "2001:db8::10" { t.Fatalf("public IP=%q", got) }
	if cfg.AdminPassword != "secret-password" { t.Fatal("admin password was not loaded") }
}

func TestLoadStillAcceptsIPv4(t *testing.T) {
	t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
	t.Setenv("SHIFT_MY_PUBLIC_IP", "203.0.113.10")
	t.Setenv("SHIFT_MY_ADMIN_PASSWORD", "secret-password")
	cfg, err := Load()
	if err != nil { t.Fatal(err) }
	if got := cfg.PublicIP.String(); got != "203.0.113.10" { t.Fatalf("public IP=%q", got) }
}
