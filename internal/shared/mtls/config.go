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

	caPool, err := loadCAPool(caPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ServerConfig builds a tls.Config for a mTLS server. It loads the server
// certificate and key, builds a CA pool from caPath for verifying client
// certs, and requires TLS 1.3 + an authenticated client.
func ServerConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	serverCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load server keypair from %s: %w", certPath, err)
	}

	caPool, err := loadCAPool(caPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func loadCAPool(caPath string) (*x509.CertPool, error) {
	caBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read ca bundle %s: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("no valid PEM certs found in %s", caPath)
	}
	return pool, nil
}
