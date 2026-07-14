package grpc

import (
	"context"
	"crypto/ecdh"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	streamv1 "nbox/gen/stream/v1"
	"nbox/internal/auth"
	"nbox/internal/entrypushd/nboxclient"
	"nbox/internal/entrypushd/vault"
	"nbox/internal/event"
)

// Snapshotter is the subset of *nbox.Client that StreamServer needs.
// Lets tests inject a fake without an HTTP server.
type Snapshotter interface {
	Snapshot(ctx context.Context, token, prefix string) ([]nboxclient.Entry, error)
	Resolve(ctx context.Context, token, key string) (string, error)
}

// StreamServer implements streamv1.KVStreamServer. Each Watch invocation
// subscribes to the Broker, optionally sends a snapshot of current state
// (read from nbox via HTTP), then forwards every matching delta to the
// gRPC stream until the client disconnects.
type StreamServer struct {
	streamv1.UnimplementedKVStreamServer
	broker            *Broker
	snapshot          Snapshotter // nil => snapshot disabled (deltas only)
	logger            *slog.Logger
	sf                singleflight.Group // dedupes concurrent same-event vault resolves
	keepaliveInterval time.Duration      // 0 => heartbeat disabled
}

// KeepaliveInterval is the heartbeat period as a named type so fx can
// wire it unambiguously (same pattern as ListenAddr/HMACKey). 0 disables.
type KeepaliveInterval time.Duration

// NewStreamServer builds a StreamServer backed by the given Broker. If
// snapshot is nil, Watch skips the snapshot phase and streams deltas only
// (retro-compatible with deployments without ENTRYPUSHD_NBOX_URL).
func NewStreamServer(broker *Broker, snapshot Snapshotter, logger *slog.Logger, keepalive KeepaliveInterval) *StreamServer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &StreamServer{
		broker:            broker,
		snapshot:          snapshot,
		logger:            logger.With(slog.String("component", "stream-server")),
		keepaliveInterval: time.Duration(keepalive),
	}
}

// Watch is the server-streaming RPC. It subscribes first (so concurrent
// deltas buffer), emits the snapshot, then drains the subscriber channel.
// On snapshot failure it returns codes.Unavailable (fail-closed) so the
// agent reconnects rather than operating on incomplete state.
func (s *StreamServer) Watch(req *streamv1.WatchRequest, stream streamv1.KVStream_WatchServer) error {
	filter := Filter{Prefixes: req.Prefixes, Types: req.Types}
	id, ch := s.broker.Subscribe(filter) // (1) deltas start buffering HERE
	defer s.broker.Unsubscribe(id)

	pub, nonce, _ := clientKeyFromContext(stream.Context())
	token, _ := auth.M2MTokenFromContext(stream.Context())

	// Derive a stable, non-forgeable client_id for audit when the agent
	// presents a vault pubkey (binds subject + instance nonce + pubkey).
	clientID := ""
	if pub != nil {
		subject := ""
		if ident, ok := auth.IdentityFromContext(stream.Context()); ok {
			subject = ident.Subject
		}
		clientID = vault.DeriveClientID(subject, nonce, pub)
	}

	s.logger.Info("client connected",
		slog.String("id", id),
		slog.String("client_id", clientID),
		slog.Any("prefixes", req.Prefixes),
		slog.Any("types", req.Types),
	)
	defer s.logger.Info("client disconnected", slog.String("id", id))

	// (2) snapshot-on-connect: only when a client is configured and the
	// request scopes prefixes (no full-tree fetch).
	if s.snapshot != nil && len(req.Prefixes) > 0 {
		if err := s.sendSnapshot(stream, req.Prefixes, pub, token); err != nil {
			return err
		}
	}

	// Heartbeat: keeps the stream's L7 path (ALB) from idle-killing it.
	// Starts after the snapshot burst (already continuous DATA). A nil
	// channel (disabled) is an inert select case.
	var keepalive <-chan time.Time
	if s.keepaliveInterval > 0 {
		t := time.NewTicker(s.keepaliveInterval)
		defer t.Stop()
		keepalive = t.C
	}

	// (3) drain live deltas (including any buffered during the snapshot).
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-keepalive:
			if err := stream.Send(keepaliveEvent()); err != nil {
				return fmt.Errorf("send keepalive: %w", err)
			}
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			if pub != nil && evt.Type == string(event.EntryUpserted) && isVaultKey(evt.Subject) {
				plaintext, err := s.resolveOnce(stream.Context(), evt.Id, token, evt.Subject)
				if err != nil {
					s.logger.Error("vault resolve failed on delta, failing closed", slog.String("key", evt.Subject), slog.Any("error", err))
					return status.Errorf(codes.Unavailable, "vault resolve failed: %v", err)
				}
				sealed, err := s.sealValue(evt.Subject, plaintext, pub)
				if err != nil {
					s.logger.Error("vault seal failed on delta, failing closed", slog.String("key", evt.Subject), slog.Any("error", err))
					return status.Errorf(codes.Unavailable, "vault seal failed: %v", err)
				}
				sealed.TimeUnixMs = evt.TimeUnixMs // preserve ingress time across the reseal
				evt = sealed
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		}
	}
}

// sendSnapshot fetches each prefix's current state and sends it as a
// burst of EntryUpserted events. Fail-closed on any fetch or seal error.
func (s *StreamServer) sendSnapshot(stream streamv1.KVStream_WatchServer, prefixes []string, pub *ecdh.PublicKey, token string) error {
	for _, prefix := range prefixes {
		entries, err := s.snapshot.Snapshot(stream.Context(), token, prefix)
		if err != nil {
			s.logger.Error("snapshot fetch failed, failing closed", slog.String("prefix", prefix), slog.Any("error", err))
			return status.Errorf(codes.Unavailable, "snapshot failed: %v", err)
		}
		for i := range entries {
			var evt *streamv1.Event
			if pub != nil && isVaultKey(entries[i].Key) {
				evt, err = s.sealFor(stream.Context(), entries[i].Key, token, pub)
				if err != nil {
					s.logger.Error("vault seal failed, failing closed", slog.String("key", entries[i].Key), slog.Any("error", err))
					return status.Errorf(codes.Unavailable, "vault seal failed: %v", err)
				}
			} else {
				evt = toEvent(&entries[i])
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		}
		s.logger.Info("snapshot sent", slog.String("prefix", prefix), slog.Int("entries", len(entries)))
	}
	return nil
}

// toEvent maps a snapshot Entry to an EntryUpserted event. The snapshot
// is emitted as a burst of upserts (= current state); agents apply
// upsert as "set this key" with last-write-wins by Subject, so a key
// appearing both in the snapshot and as a concurrent delta is idempotent.
func toEvent(e *nboxclient.Entry) *streamv1.Event {
	data, _ := json.Marshal(e) // Entry is always JSON-serializable.
	return &streamv1.Event{
		Id:      uuid.NewString(),
		Type:    string(event.EntryUpserted),
		Source:  "nbox",
		Subject: e.Key,
		Data:    data,
	}
}

// keepaliveEvent builds the empty heartbeat event. One DATA frame is all
// the ALB needs to reset its idle timer.
func keepaliveEvent() *streamv1.Event {
	return &streamv1.Event{
		Id:         uuid.NewString(),
		Type:       string(event.Keepalive),
		Source:     "nbox",
		TimeUnixMs: time.Now().UnixMilli(),
	}
}

// vaultPrefix is the key prefix whose entries are delivered HPKE-sealed.
// passbox is the vault prefix (every entry is secure). Could be sourced
// from prefix-config (isVault) later; hardcoded per Fase 3 scope.
const vaultPrefix = "passbox/"

func isVaultKey(subject string) bool { return strings.HasPrefix(subject, vaultPrefix) }

// sealFor resolves a vault key's plaintext and seals it to pub, returning
// an EntryUpserted event carrying the HPKE ciphertext. Used by the snapshot
// (per key). The delta path uses resolveOnce + sealValue instead. Fail-closed:
// any resolve/seal error is returned to the caller (which aborts Watch).
func (s *StreamServer) sealFor(ctx context.Context, subject, token string, pub *ecdh.PublicKey) (*streamv1.Event, error) {
	plaintext, err := s.snapshot.Resolve(ctx, token, subject)
	if err != nil {
		return nil, err
	}
	return s.sealValue(subject, plaintext, pub)
}

// sealValue HPKE-seals an already-resolved plaintext to pub, returning an
// EntryUpserted event carrying the ciphertext.
func (s *StreamServer) sealValue(subject, plaintext string, pub *ecdh.PublicKey) (*streamv1.Event, error) {
	sealed, err := vault.Seal(pub, subject, []byte(plaintext))
	if err != nil {
		return nil, err
	}
	return &streamv1.Event{
		Id:         uuid.NewString(),
		Type:       string(event.EntryUpserted),
		Source:     "nbox",
		Subject:    subject,
		Data:       sealed,
		Extensions: map[string]string{"encrypted": "hpke", "suite_id": "1", "key_fpr": vault.Fingerprint(pub)},
	}, nil
}

// resolveOnce resolves a vault key's plaintext, deduping the N per-subscriber
// goroutines that process the SAME broadcast event via singleflight (key =
// eventID|subject). First caller resolves; the rest share the result; the key
// is forgotten after. A new event id resolves fresh (no cross-event cache).
func (s *StreamServer) resolveOnce(ctx context.Context, eventID, token, subject string) (string, error) {
	v, err, _ := s.sf.Do(eventID+"|"+subject, func() (any, error) {
		return s.snapshot.Resolve(ctx, token, subject)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}
