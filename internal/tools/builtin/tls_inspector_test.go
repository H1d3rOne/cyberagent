package builtin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTLSInspect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := TLSInspect(ctx, TLSInspectOptions{URL: server.URL, Timeout: 5 * time.Second}, "job_001", "step_tls_001")

	if !result.Connected {
		t.Fatalf("expected connected, got error: %s", result.Error)
	}
	if result.Protocol == "" {
		t.Error("expected protocol to be set")
	}
	if result.CipherSuite == "" {
		t.Error("expected cipher suite to be set")
	}
	if len(result.Certificates) == 0 {
		t.Error("expected at least one certificate")
	}
	if len(result.Evidence) == 0 {
		t.Error("expected evidence to be generated")
	}
}

func TestTLSInspectExpiredCert(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired.test"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			tlsConn := conn.(*tls.Conn)
			_ = tlsConn.Handshake()
			tlsConn.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := "https://" + listener.Addr().String()
	result := TLSInspect(ctx, TLSInspectOptions{URL: url, Timeout: 5 * time.Second}, "job_002", "step_tls_002")

	if !result.Connected {
		t.Fatalf("expected connected, got error: %s", result.Error)
	}
	if len(result.Certificates) == 0 {
		t.Fatal("expected certificates")
	}
	if !result.Certificates[0].Expired {
		t.Error("expected certificate to be marked expired")
	}
	foundExpiredFinding := false
	for _, f := range result.Findings {
		if f.Category == "tls/certificate" {
			foundExpiredFinding = true
		}
	}
	if !foundExpiredFinding {
		t.Error("expected expired certificate finding")
	}
}

func TestTLSInspectInvalidURL(t *testing.T) {
	ctx := context.Background()
	result := TLSInspect(ctx, TLSInspectOptions{URL: "not-a-url", Timeout: 2 * time.Second}, "job_003", "step_tls_003")
	if result.Connected {
		t.Error("expected not connected for invalid URL")
	}
}
