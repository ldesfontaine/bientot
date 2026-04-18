package echoserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type Server struct {
	addr    string
	cert    string
	key     string
	caCerts string
	log     *slog.Logger
}

func New(log *slog.Logger, addr, certPath, keyPath, caPath string) *Server {
	return &Server{
		addr:    addr,
		cert:    certPath,
		key:     keyPath,
		caCerts: caPath,
		log:     log,
	}
}

func (s *Server) Run(ctx context.Context) error {
	serverCert, err := tls.LoadX509KeyPair(s.cert, s.key)
	if err != nil {
		return fmt.Errorf("load server keypair: %w", err)
	}

	caBytes, err := os.ReadFile(s.caCerts)
	if err != nil {
		return fmt.Errorf("read ca bundle: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return fmt.Errorf("append ca certs from %s: no valid PEM found", s.caCerts)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", s.handlePing)

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.log.Error("shutdown error", "err", err)
		}
	}()

	s.log.Info("echo-server listening", "addr", s.addr)
	if err := srv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "no client cert", http.StatusUnauthorized)
		return
	}

	clientCN := r.TLS.PeerCertificates[0].Subject.CommonName

	s.log.Info("request",
		"method", r.Method,
		"path", r.URL.Path,
		"remote", r.RemoteAddr,
		"client_cn", clientCN,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"from":      "echo",
		"client_cn": clientCN,
	})
}
