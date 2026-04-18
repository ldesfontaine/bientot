// Package mtls builds tls.Config values for mutual TLS clients and servers.
package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// ClientConfig builds a tls.Config for a mTLS client. It loads the client
// certificate and key from disk, builds a CA pool from caPath for verifying
// the server certificate, and sets ServerName to verify the server's SAN.
//
// TLS 1.3 is enforced. InsecureSkipVerify is NEVER set.
func ClientConfig(certPath, keyPath, caPath, serverName string) (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client keypair from %s: %w", certPath, err)
	}

	caBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read ca bundle %s: %w", caPath, err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("no valid PEM certs found in %s", caPath)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
