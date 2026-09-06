package app

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type Config struct {
	DataDir   string
	Address   string
	TLSCert   string
	TLSKey    string
	TLSCA     string
	PublicURL string
	Dev       bool
}

func (c Config) Port() string {
	_, port, err := net.SplitHostPort(c.Address)
	if err == nil && port != "" {
		return port
	}
	return "8787"
}

func (c Config) Scheme() string {
	if c.TLSCert != "" {
		return "https"
	}
	return "http"
}

func (c Config) LoopbackURL() string { return c.Scheme() + "://127.0.0.1:" + c.Port() }

func (c Config) MobileURL(host string) string {
	if c.PublicURL != "" {
		return c.PublicURL
	}
	return c.Scheme() + "://" + host + ":" + c.Port()
}

func LoadConfig(dev bool) (Config, error) {
	dataDir := os.Getenv("SENTRYMED_DATA_DIR")
	if dataDir == "" {
		if dev {
			dataDir = "./data"
		} else {
			userConfig, err := os.UserConfigDir()
			if err != nil {
				return Config{}, fmt.Errorf("resolve config directory: %w", err)
			}
			dataDir = filepath.Join(userConfig, "SentryMed")
		}
	}
	abs := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		resolved, err := filepath.Abs(path)
		if err != nil {
			return path
		}
		return resolved
	}
	dataDir = abs(dataDir)
	for _, directory := range []string{"database", "documents", "backups", "logs"} {
		if err := os.MkdirAll(filepath.Join(dataDir, directory), 0o750); err != nil {
			return Config{}, fmt.Errorf("create %s directory: %w", directory, err)
		}
	}
	address := os.Getenv("SENTRYMED_ADDRESS")
	if address == "" {
		address = ":8787"
	}
	tlsCert, tlsKey := os.Getenv("SENTRYMED_TLS_CERT"), os.Getenv("SENTRYMED_TLS_KEY")
	if (tlsCert == "") != (tlsKey == "") {
		return Config{}, fmt.Errorf("SENTRYMED_TLS_CERT and SENTRYMED_TLS_KEY must be configured together")
	}
	tlsCA := os.Getenv("SENTRYMED_TLS_CA")
	if tlsCert == "" && !dev && os.Getenv("SENTRYMED_AUTO_TLS") != "false" {
		var err error
		tlsCert, tlsKey, tlsCA, err = ensureLocalTLS(dataDir)
		if err != nil {
			return Config{}, fmt.Errorf("prepare local HTTPS: %w", err)
		}
	}
	return Config{DataDir: dataDir, Address: address, TLSCert: tlsCert, TLSKey: tlsKey, TLSCA: tlsCA, PublicURL: os.Getenv("SENTRYMED_PUBLIC_URL"), Dev: dev}, nil
}
