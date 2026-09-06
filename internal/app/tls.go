package app

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func ensureLocalTLS(dataDir string) (string, string, string, error) {
	directory := filepath.Join(dataDir, "certificates")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", "", err
	}
	caCertPath, caKeyPath := filepath.Join(directory, "sentrymed-local-ca.crt"), filepath.Join(directory, "sentrymed-local-ca.key")
	serverCertPath, serverKeyPath := filepath.Join(directory, "sentrymed-server.crt"), filepath.Join(directory, "sentrymed-server.key")
	caCert, caKey, err := loadOrCreateCA(caCertPath, caKeyPath)
	if err != nil {
		return "", "", "", err
	}
	if err := createServerCertificate(serverCertPath, serverKeyPath, caCert, caKey); err != nil {
		return "", "", "", err
	}
	return serverCertPath, serverKeyPath, caCertPath, nil
}

func loadOrCreateCA(certPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		certBlock, _ := pem.Decode(certPEM)
		keyBlock, _ := pem.Decode(keyPEM)
		if certBlock != nil && keyBlock != nil {
			certificate, certParseErr := x509.ParseCertificate(certBlock.Bytes)
			key, keyParseErr := x509.ParseECPrivateKey(keyBlock.Bytes)
			if certParseErr == nil && keyParseErr == nil && time.Now().Before(certificate.NotAfter) {
				return certificate, key, nil
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	template := &x509.Certificate{SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: "SentryMed Opti Local CA", Organization: []string{"SentryMed Opti"}}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err := writePEM(certPath, 0o644, "CERTIFICATE", der); err != nil {
		return nil, nil, err
	}
	if err := writePEM(keyPath, 0o600, "EC PRIVATE KEY", keyDER); err != nil {
		return nil, nil, err
	}
	certificate, err := x509.ParseCertificate(der)
	return certificate, key, err
}

func createServerCertificate(certPath, keyPath string, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	interfaces, _ := net.InterfaceAddrs()
	for _, address := range interfaces {
		if ip, _, err := net.ParseCIDR(address.String()); err == nil && !ip.IsLoopback() {
			ips = append(ips, ip)
		}
	}
	template := &x509.Certificate{SerialNumber: randomSerial(), Subject: pkix.Name{CommonName: "sentrymed.local", Organization: []string{"SentryMed Opti"}}, DNSNames: []string{"sentrymed.local", "localhost"}, IPAddresses: ips, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(2, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := writePEM(certPath, 0o644, "CERTIFICATE", der); err != nil {
		return err
	}
	return writePEM(keyPath, 0o600, "EC PRIVATE KEY", keyDER)
}

func randomSerial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return serial
}

func writePEM(path string, mode os.FileMode, kind string, data []byte) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if err := pem.Encode(file, &pem.Block{Type: kind, Bytes: data}); err != nil {
		_ = file.Close()
		_ = os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("install certificate: %w", err)
	}
	return nil
}
