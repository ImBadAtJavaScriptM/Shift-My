package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

type Config struct {
	PublicHost     string
	PublicIPv4     net.IP
	AdminPassword  string
	DBPath         string
	CACertPath     string
	CAKeyPath      string
	PublicCertPath string
	PublicKeyPath  string
	CaptureEnabled bool
	CaptureDir     string
}

func Load() (Config, error) {
	host := os.Getenv("SHIFT_MY_PUBLIC_HOST")
	ip := net.ParseIP(os.Getenv("SHIFT_MY_PUBLIC_IPV4"))
	adminPassword := os.Getenv("SHIFT_MY_ADMIN_PASSWORD")
	if host == "" || ip == nil || ip.To4() == nil || adminPassword == "" {
		return Config{}, fmt.Errorf("SHIFT_MY_PUBLIC_HOST, valid SHIFT_MY_PUBLIC_IPV4, and SHIFT_MY_ADMIN_PASSWORD are required")
	}
	capture, err := strconv.ParseBool(envDefault("SHIFT_MY_CAPTURE_ENABLED", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("capture flag: %w", err)
	}
	return Config{
		PublicHost:     host,
		PublicIPv4:     ip.To4(),
		AdminPassword:  adminPassword,
		DBPath:         envDefault("SHIFT_MY_DB_PATH", "./shift-my.db"),
		CACertPath:     envDefault("SHIFT_MY_CA_CERT", "./certs/root-ca.pem"),
		CAKeyPath:      envDefault("SHIFT_MY_CA_KEY", "./certs/root-ca-key.pem"),
		PublicCertPath: envDefault("SHIFT_MY_PUBLIC_CERT", "./certs/public.pem"),
		PublicKeyPath:  envDefault("SHIFT_MY_PUBLIC_KEY", "./certs/public-key.pem"),
		CaptureEnabled: capture,
		CaptureDir:     envDefault("SHIFT_MY_CAPTURE_DIR", "./captures"),
	}, nil
}

func envDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
