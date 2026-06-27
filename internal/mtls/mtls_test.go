package mtls

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempPEMPair(t testing.TB, dir, prefix string) (certPEM, keyPEM, caCertPEM string) {
	t.Helper()
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(0),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPub, caPriv)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(dir, prefix+"_ca.crt")
	cf, err := os.Create(caPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cf.Close()
	pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test Server"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caTmpl, pub, caPriv)
	if err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(dir, prefix+".crt")
	cf2, err := os.Create(certPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cf2.Close()
	pem.Encode(cf2, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyPath := filepath.Join(dir, prefix+".key")
	kf, err := os.Create(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer kf.Close()
	b, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pem.Encode(kf, &pem.Block{Type: "PRIVATE KEY", Bytes: b})

	return certPath, keyPath, caPath
}

func TestNewServerConfig_RequiresAndVerifiesClientCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := tempPEMPair(t, dir, "server")
	cfg, err := NewServerConfig(certPath, keyPath, caPath)
	if err != nil {
		t.Fatalf("NewServerConfig: %v", err)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS 1.3, got %x", cfg.MinVersion)
	}
}
