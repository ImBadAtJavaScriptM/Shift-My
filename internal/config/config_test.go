package config

import "testing"

func TestLoadRequiresPublicHostAndIPv4(t *testing.T) {
    t.Setenv("SHIFT_MY_PUBLIC_HOST", "")
    t.Setenv("SHIFT_MY_PUBLIC_IPV4", "")
    if _, err := Load(); err == nil {
        t.Fatal("expected missing host/IP error")
    }
}

func TestLoadAcceptsValidIPv4(t *testing.T) {
    t.Setenv("SHIFT_MY_PUBLIC_HOST", "test.example.com")
    t.Setenv("SHIFT_MY_PUBLIC_IPV4", "203.0.113.10")
    t.Setenv("SHIFT_MY_DB_PATH", t.TempDir()+"/test.db")
    t.Setenv("SHIFT_MY_CA_CERT", "/tmp/ca.pem")
    t.Setenv("SHIFT_MY_CA_KEY", "/tmp/ca-key.pem")
    t.Setenv("SHIFT_MY_PUBLIC_CERT", "/tmp/fullchain.pem")
    t.Setenv("SHIFT_MY_PUBLIC_KEY", "/tmp/privkey.pem")
    t.Setenv("SHIFT_MY_CAPTURE_ENABLED", "false")
    cfg, err := Load()
    if err != nil { t.Fatal(err) }
    if cfg.PublicHost != "test.example.com" { t.Fatalf("host=%q", cfg.PublicHost) }
}
