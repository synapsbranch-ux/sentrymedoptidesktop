package app

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// ClientTLS reports whether clients reach this server over TLS: either this
// process terminates it directly, or a trusted co-located reverse proxy does
// and the public HTTPS origin is configured (see server.secureRequest). This
// is deliberately separate from Scheme(), which describes what THIS process's
// own loopback listener speaks (still plain HTTP when a proxy holds the
// certificate) and must keep driving LoopbackURL().
func (c Config) ClientTLS() bool {
	return c.TLSCert != "" || strings.HasPrefix(c.PublicURL, "https://")
}

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
	for _, directory := range []string{"database", "documents", "images", "insurance-cards", "insurance-documents", "backups", "logs"} {
		if err := os.MkdirAll(filepath.Join(dataDir, directory), 0o750); err != nil {
			return Config{}, fmt.Errorf("create %s directory: %w", directory, err)
		}
	}
	address := os.Getenv("SENTRYMED_ADDRESS")
	if address == "" {
		if dev {
			address = "127.0.0.1:8787"
		} else {
			address = ":8787"
		}
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
	if tlsCert == "" && !dev {
		host, _, err := net.SplitHostPort(address)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			return Config{}, fmt.Errorf("unencrypted production HTTP may only bind to loopback; configure TLS for LAN access")
		}
	}
	publicURL := strings.TrimRight(os.Getenv("SENTRYMED_PUBLIC_URL"), "/")
	if publicURL != "" {
		parsed, err := url.Parse(publicURL)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || (parsed.Scheme != "https" && !(dev && parsed.Scheme == "http")) {
			return Config{}, fmt.Errorf("SENTRYMED_PUBLIC_URL must be an HTTPS origin without credentials, path, query or fragment")
		}
	}
	return Config{DataDir: dataDir, Address: address, TLSCert: tlsCert, TLSKey: tlsKey, TLSCA: tlsCA, PublicURL: publicURL, Dev: dev}, nil
}
