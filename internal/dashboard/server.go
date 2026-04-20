package dashboardsrv

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
	"github.com/ldesfontaine/bientot/internal/dashboard/nonce"
	"github.com/ldesfontaine/bientot/internal/dashboard/storage"
	"github.com/ldesfontaine/bientot/internal/shared/crypto"
	"github.com/ldesfontaine/bientot/internal/shared/keys"
	"github.com/ldesfontaine/bientot/internal/shared/mtls"
)

const (
	maxPayloadSize  = 1 << 20
	timestampSkew   = 60 * time.Second
	nonceTTL        = 5 * time.Minute
	nonceEvictEvery = 1 * time.Minute
	protocolVersion = 1
)

type Server struct {
	addr      string
	cert      string
	key       string
	caCerts   string
	agentKeys string
	pubKeys   map[string]ed25519.PublicKey
	nonces    *nonce.Cache
	db        *storage.Storage
	log       *slog.Logger
}

func New(log *slog.Logger, addr, certPath, keyPath, caPath, agentKeysDir string, db *storage.Storage) *Server {
	return &Server{
		addr:      addr,
		cert:      certPath,
		key:       keyPath,
		caCerts:   caPath,
		agentKeys: agentKeysDir,
		db:        db,
		log:       log,
	}
}

func (s *Server) Run(ctx context.Context) error {
	tlsConfig, err := mtls.ServerConfig(s.cert, s.key, s.caCerts)
	if err != nil {
		return fmt.Errorf("build tls config: %w", err)
	}

	pubKeys, err := keys.LoadPublicKeysDir(s.agentKeys)
	if err != nil {
		return fmt.Errorf("load agent keys: %w", err)
	}
	s.pubKeys = pubKeys
	s.log.Info("agent keys loaded", "count", len(pubKeys))

	s.nonces = nonce.NewCache(nonceTTL)
	evictCtx, evictCancel := context.WithCancel(ctx)
	defer evictCancel()
	go s.nonces.Evict(evictCtx, nonceEvictEvery)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", s.handlePing)
	mux.HandleFunc("/v1/push", s.handlePush)

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

	s.log.Info("dashboard listening", "addr", s.addr)
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
		"from":      "dashboard",
		"client_cn": clientCN,
	})
}

func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadSize))
	if err != nil {
		s.log.Warn("read body failed", "error", err)
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}

	var req bientotv1.PushRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		s.log.Warn("unmarshal failed", "error", err)
		http.Error(w, "invalid proto", http.StatusBadRequest)
		return
	}

	if req.V != protocolVersion {
		s.log.Warn("bad protocol version", "got", req.V, "want", protocolVersion)
		http.Error(w, "bad version", http.StatusBadRequest)
		return
	}

	pushTime := time.Unix(0, req.TimestampNs)
	skew := time.Since(pushTime)
	if skew > timestampSkew || skew < -timestampSkew {
		s.log.Warn("stale timestamp", "skew", skew, "machine_id", req.MachineId)
		http.Error(w, "stale timestamp", http.StatusBadRequest)
		return
	}

	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "no client cert", http.StatusUnauthorized)
		return
	}
	tlsCN := r.TLS.PeerCertificates[0].Subject.CommonName
	if tlsCN != req.MachineId {
		s.log.Warn("machine_id mismatch",
			"tls_cn", tlsCN,
			"payload_machine_id", req.MachineId,
		)
		http.Error(w, "machine_id mismatch", http.StatusForbidden)
		return
	}

	pubKey, ok := s.pubKeys[req.MachineId]
	if !ok {
		s.log.Warn("unknown agent", "machine_id", req.MachineId)
		http.Error(w, "unknown agent", http.StatusUnauthorized)
		return
	}

	if err := crypto.Verify(&req, pubKey); err != nil {
		s.log.Warn("signature invalid", "machine_id", req.MachineId, "error", err)
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	}

	if !s.nonces.CheckAndAdd(req.Nonce, time.Now()) {
		s.log.Warn("replay detected", "nonce", req.Nonce, "machine_id", req.MachineId)
		http.Error(w, "replay detected", http.StatusConflict)
		return
	}

	if err := s.db.SavePush(r.Context(), &req); err != nil {
		s.log.Error("save push failed", "machine_id", req.MachineId, "error", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	totalMetrics := 0
	for _, m := range req.Modules {
		totalMetrics += len(m.Metrics)
	}

	s.log.Info("push accepted",
		"machine_id", req.MachineId,
		"modules", len(req.Modules),
		"metrics", totalMetrics,
		"nonce", req.Nonce,
	)

	resp := &bientotv1.PushResponse{
		Status: "ok",
		// gosec G115: int→int32 conversions are bounded by maxPayloadSize (1 MB),
		// which caps the decoded proto well below math.MaxInt32.
		AcceptedModules: int32(len(req.Modules)), //nolint:gosec
		AcceptedMetrics: int32(totalMetrics),     //nolint:gosec
	}
	respBytes, err := proto.Marshal(resp)
	if err != nil {
		s.log.Error("marshal response", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}
