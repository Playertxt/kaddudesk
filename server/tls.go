package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var certFingerprint string // SHA-256 of the DER certificate, uppercase hex

// loadOrCreateCert returns a persistent self-signed certificate. Clients pin its
// fingerprint on first connection (trust-on-first-use), so traffic is encrypted
// and a man-in-the-middle is detected afterwards.
func loadOrCreateCert() (tls.Certificate, error) {
	certFile := filepath.Join(dataDir, "cert.pem")
	keyFile := filepath.Join(dataDir, "key.pem")
	if c, err := tls.LoadX509KeyPair(certFile, keyFile); err == nil {
		setFingerprint(c.Certificate[0])
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	host, _ := os.Hostname()
	tpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "KadduDesk " + host, Organization: []string{"KadduDesk"}},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().AddDate(20, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", host},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	_ = os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
	setFingerprint(der)
	return tls.LoadX509KeyPair(certFile, keyFile)
}

func setFingerprint(der []byte) {
	s := sha256.Sum256(der)
	certFingerprint = strings.ToUpper(hex.EncodeToString(s[:]))
}

// prettyFingerprint groups the fingerprint for humans: "AB12 CD34 ...", first 8 groups.
func prettyFingerprint() string {
	f := certFingerprint
	var parts []string
	for i := 0; i+4 <= len(f) && len(parts) < 8; i += 4 {
		parts = append(parts, f[i:i+4])
	}
	return strings.Join(parts, " ")
}
