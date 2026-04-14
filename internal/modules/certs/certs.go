package certs

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/ldesfontaine/bientot/internal/transport"
)

// Module vérifie l'expiration des certificats TLS pour les domaines configurés.
type Module struct {
	domains []string
}

func New(domains []string) *Module {
	return &Module{domains: domains}
}

func (m *Module) Name() string { return "certificates" }

func (m *Module) Detect() bool {
	return len(m.domains) > 0
}

func (m *Module) Collect(_ context.Context) (transport.ModuleData, error) {
	now := time.Now()
	var metrics []transport.MetricPoint
	metadata := make(map[string]string)

	for _, domain := range m.domains {
		labels := map[string]string{"domain": domain}

		info, err := checkCert(domain)
		if err != nil {
			metrics = append(metrics, transport.MetricPoint{
				Name: "cert_valid", Value: 0, Labels: labels,
			})
			metadata["error_"+domain] = err.Error()
			continue
		}

		daysLeft := time.Until(info.notAfter).Hours() / 24
		metrics = append(metrics,
			transport.MetricPoint{Name: "cert_valid", Value: 1, Labels: labels},
			transport.MetricPoint{Name: "cert_expiry_days", Value: daysLeft, Labels: labels},
		)
		metadata["issuer_"+domain] = info.issuer
		metadata["subject_"+domain] = info.subject
	}

	return transport.ModuleData{
		Module:    "certificates",
		Metrics:   metrics,
		Metadata:  metadata,
		Timestamp: now,
	}, nil
}

type certInfo struct {
	subject  string
	issuer   string
	notAfter time.Time
}

func checkCert(domain string) (*certInfo, error) {
	host := domain
	if _, _, err := net.SplitHostPort(domain); err != nil {
		host = domain + ":443"
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
		InsecureSkipVerify: false,
	})
	if err != nil {
		return nil, fmt.Errorf("connexion TLS %s : %w", domain, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("aucun certificat depuis %s", domain)
	}

	leaf := certs[0]
	return &certInfo{
		subject:  leaf.Subject.CommonName,
		issuer:   leaf.Issuer.CommonName,
		notAfter: leaf.NotAfter,
	}, nil
}
