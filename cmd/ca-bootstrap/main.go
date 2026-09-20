package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ImBadAtJavaScriptM/Shift-My/internal/pki"
)

func main() {
	out := flag.String("out", "./certs", "output directory")
	prefix := flag.String("prefix", "root-ca", "output filename prefix")
	commonName := flag.String("common-name", "Shift-My Test Root CA", "root certificate common name")
	force := flag.Bool("force", false, "overwrite an existing CA key")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o700); err != nil {
		fatal(err)
	}
	if *prefix == "" || filepath.Base(*prefix) != *prefix {
		fatal(fmt.Errorf("prefix must be a simple filename"))
	}
	certPath := filepath.Join(*out, *prefix+".pem")
	keyPath := filepath.Join(*out, *prefix+"-key.pem")
	if !*force {
		if _, err := os.Stat(keyPath); err == nil {
			fatal(fmt.Errorf("refusing to overwrite existing CA key %s; use -force explicitly", keyPath))
		} else if !os.IsNotExist(err) {
			fatal(err)
		}
	}

	certPEM, keyPEM, err := pki.GenerateRoot(*commonName)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		fatal(err)
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		fatal(err)
	}
	fmt.Printf("created %s and %s\n", certPath, keyPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "ca-bootstrap:", err)
	os.Exit(1)
}
