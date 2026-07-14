package grpc

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	streamv1 "nbox/gen/stream/v1"
	"nbox/internal/auth"
	"nbox/internal/entrypushd/nboxclient"
	"nbox/internal/entrypushd/vault"
	"nbox/internal/event"
)

func TestToEvent_MapsEntryToUpsert(t *testing.T) {
	e := nboxclient.Entry{Key: "passbox/banking/api", Value: "secret-val", Secure: true}

	evt := toEvent(&e)

	require.Equal(t, "passbox/banking/api", evt.Subject)
	require.Equal(t, string(event.EntryUpserted), evt.Type)
	require.Equal(t, "nbox", evt.Source)
	require.NotEmpty(t, evt.Id, "Id should be a non-empty uuid")

	var got nboxclient.Entry
	require.NoError(t, json.Unmarshal(evt.Data, &got))
	require.Equal(t, e, got, "Data carries the entry JSON")
}

// fakeSnap is a test double for the snapshotter interface.
type fakeSnap struct {
	entries     []nboxclient.Entry
	err         error
	gotTok      string
	gotPfx      []string
	mu          sync.Mutex
	resolveVals map[string]string
	resolveErr  error
}

func (f *fakeSnap) Snapshot(_ context.Context, token, prefix string) ([]nboxclient.Entry, error) {
	f.mu.Lock()
	f.gotTok = token
	f.gotPfx = append(f.gotPfx, prefix)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

func (f *fakeSnap) Resolve(_ context.Context, _, key string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	return f.resolveVals[key], nil
}

// mockStream is a minimal streamv1.KVStream_WatchServer that records
// sent events and exposes a cancelable context.
type mockStream struct {
	streamv1.KVStream_WatchServer
	ctx  context.Context
	sent []*streamv1.Event
	mu   sync.Mutex
}

func (m *mockStream) Context() context.Context { return m.ctx }
func (m *mockStream) Send(e *streamv1.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, e)
	return nil
}

func (m *mockStream) snapshotOf() []*streamv1.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*streamv1.Event, len(m.sent))
	copy(out, m.sent)
	return out
}

func TestWatch_SnapshotBeforeDeltas(t *testing.T) {
	broker := NewBroker(nil)
	snap := &fakeSnap{entries: []nboxclient.Entry{{Key: "p/a", Value: "1"}, {Key: "p/b", Value: "2"}}}
	srv := NewStreamServer(broker, snap, nil, 0)

	ctx, cancel := context.WithCancel(auth.WithM2MToken(context.Background(), "jwt-xyz"))
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"p/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()

	require.Eventually(t, func() bool { return len(stream.snapshotOf()) == 2 }, time.Second, 5*time.Millisecond)
	broker.Publish(&streamv1.Event{Id: "d1", Type: string(event.EntryUpserted), Subject: "p/c"})
	require.Eventually(t, func() bool { return len(stream.snapshotOf()) == 3 }, time.Second, 5*time.Millisecond)

	cancel()
	require.NoError(t, <-done)

	got := stream.snapshotOf()
	require.Equal(t, "jwt-xyz", snap.gotTok)
	require.Equal(t, "p/a", got[0].Subject)
	require.Equal(t, "p/b", got[1].Subject)
	require.Equal(t, "p/c", got[2].Subject, "delta delivered after snapshot")
}

func TestWatch_BufferedDeltaDeliveredAfterSnapshot(t *testing.T) {
	broker := NewBroker(nil)
	release := make(chan struct{})
	snap := &blockingSnap{entries: []nboxclient.Entry{{Key: "p/a", Value: "1"}}, release: release}
	srv := NewStreamServer(broker, snap, nil, 0)

	ctx, cancel := context.WithCancel(auth.WithM2MToken(context.Background(), "jwt"))
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"p/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()

	require.Eventually(t, func() bool { return snap.entered() }, time.Second, 5*time.Millisecond)
	broker.Publish(&streamv1.Event{Id: "d1", Type: string(event.EntryUpserted), Subject: "p/z"})
	close(release)

	require.Eventually(t, func() bool { return len(stream.snapshotOf()) == 2 }, time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	got := stream.snapshotOf()
	require.Equal(t, "p/a", got[0].Subject, "snapshot first")
	require.Equal(t, "p/z", got[1].Subject, "buffered delta after snapshot")
}

func TestWatch_NilClientStreamsDeltasOnly(t *testing.T) {
	broker := NewBroker(nil)
	srv := NewStreamServer(broker, nil, nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"p/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()

	// Watch subscribes asynchronously and the broker is fire-and-forget
	// (no replay), so retry the publish until the subscription exists and
	// the delta lands. All retries carry the same subject, so duplicates
	// don't affect the assertion below.
	require.Eventually(t, func() bool {
		broker.Publish(&streamv1.Event{Id: "d1", Type: string(event.EntryUpserted), Subject: "p/a"})
		return len(stream.snapshotOf()) >= 1
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
	require.Equal(t, "p/a", stream.snapshotOf()[0].Subject)
}

func TestWatch_FetchErrorFailsClosed(t *testing.T) {
	broker := NewBroker(nil)
	snap := &fakeSnap{err: errors.New("nbox down")}
	srv := NewStreamServer(broker, snap, nil, 0)

	ctx := auth.WithM2MToken(context.Background(), "jwt")
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"p/"}}

	err := srv.Watch(req, stream)
	require.Error(t, err, "fail-closed: snapshot error aborts Watch")
}

func TestWatch_EmptyPrefixesSkipsSnapshot(t *testing.T) {
	broker := NewBroker(nil)
	snap := &fakeSnap{entries: []nboxclient.Entry{{Key: "p/a"}}}
	srv := NewStreamServer(broker, snap, nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: nil}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()

	// Fire-and-forget broker (no replay): retry the publish until the
	// subscription exists and the delta lands.
	require.Eventually(t, func() bool {
		broker.Publish(&streamv1.Event{Id: "d1", Type: string(event.EntryUpserted), Subject: "p/a"})
		return len(stream.snapshotOf()) >= 1
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
	snap.mu.Lock()
	require.Empty(t, snap.gotPfx, "snapshot not invoked when prefixes empty")
	snap.mu.Unlock()
}

func TestWatch_SealsVaultSnapshotEntries(t *testing.T) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	broker := NewBroker(nil)
	snap := &fakeSnap{
		entries:     []nboxclient.Entry{{Key: "passbox/banking/api", Secure: true}},
		resolveVals: map[string]string{"passbox/banking/api": "TOP-SECRET"},
	}
	srv := NewStreamServer(broker, snap, nil, 0)

	md := metadata.New(map[string]string{"x-vault-pubkey": pubB64})
	ctx, cancel := context.WithCancel(metadata.NewIncomingContext(
		auth.WithM2MToken(context.Background(), "jwt"), md))
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"passbox/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()

	require.Eventually(t, func() bool { return len(stream.snapshotOf()) == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	evt := stream.snapshotOf()[0]
	require.Equal(t, "passbox/banking/api", evt.Subject)
	require.Equal(t, "hpke", evt.Extensions["encrypted"])
	plain, err := vault.Open(priv, evt.Subject, evt.Data)
	require.NoError(t, err)
	require.Equal(t, "TOP-SECRET", string(plain))
}

func TestWatch_SealsVaultUpsertDelta_NotDelete(t *testing.T) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	broker := NewBroker(nil)
	// Empty snapshot; the value is resolved on the live delta.
	snap := &fakeSnap{resolveVals: map[string]string{"passbox/x": "DELTA-SECRET"}}
	srv := NewStreamServer(broker, snap, nil, 0)

	md := metadata.New(map[string]string{"x-vault-pubkey": pubB64})
	ctx, cancel := context.WithCancel(metadata.NewIncomingContext(
		auth.WithM2MToken(context.Background(), "jwt"), md))
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"passbox/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()

	// Vault UPSERT delta => sealed. Fire-and-forget broker: retry until it lands.
	require.Eventually(t, func() bool {
		broker.Publish(&streamv1.Event{Id: "u1", Type: string(event.EntryUpserted), Subject: "passbox/x"})
		return len(stream.snapshotOf()) >= 1
	}, time.Second, 10*time.Millisecond)

	upsert := stream.snapshotOf()[0]
	require.Equal(t, "hpke", upsert.Extensions["encrypted"], "vault upsert delta must be sealed")
	plain, err := vault.Open(priv, "passbox/x", upsert.Data)
	require.NoError(t, err)
	require.Equal(t, "DELTA-SECRET", string(plain))

	// The retry loop above may have buffered several "u1" upserts (the broker
	// is fire-and-forget). Let the stream quiesce so every buffered duplicate
	// has flushed before we publish the delete; otherwise a late duplicate
	// upsert could land after "before" is captured and masquerade as the
	// last-sent event.
	require.Eventually(t, func() bool {
		n := len(stream.snapshotOf())
		time.Sleep(20 * time.Millisecond)
		return len(stream.snapshotOf()) == n
	}, time.Second, 25*time.Millisecond)

	// Vault DELETE delta => NOT sealed (no value to resolve).
	before := len(stream.snapshotOf())
	require.Eventually(t, func() bool {
		broker.Publish(&streamv1.Event{Id: "d1", Type: string(event.EntryDeleted), Subject: "passbox/x"})
		return len(stream.snapshotOf()) > before
	}, time.Second, 10*time.Millisecond)

	sent := stream.snapshotOf()
	del := sent[len(sent)-1]
	require.Equal(t, string(event.EntryDeleted), del.Type)
	require.Empty(t, del.Extensions["encrypted"], "vault delete delta must NOT be sealed")

	cancel()
	require.NoError(t, <-done)
}

func TestWatch_NonVaultSnapshotStaysPlain(t *testing.T) {
	priv, _ := mustKey(t)
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	broker := NewBroker(nil)
	snap := &fakeSnap{entries: []nboxclient.Entry{{Key: "development/svc/x", Value: "v"}}}
	srv := NewStreamServer(broker, snap, nil, 0)

	md := metadata.New(map[string]string{"x-vault-pubkey": pubB64})
	ctx, cancel := context.WithCancel(metadata.NewIncomingContext(
		auth.WithM2MToken(context.Background(), "jwt"), md))
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"development/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()
	require.Eventually(t, func() bool { return len(stream.snapshotOf()) == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	evt := stream.snapshotOf()[0]
	require.Empty(t, evt.Extensions["encrypted"], "non-vault entry must not be sealed")
}

func TestWatch_SealResolveErrorFailsClosed(t *testing.T) {
	priv, _ := mustKey(t)
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	broker := NewBroker(nil)
	snap := &fakeSnap{
		entries:    []nboxclient.Entry{{Key: "passbox/k", Secure: true}},
		resolveErr: errors.New("nbox down"),
	}
	srv := NewStreamServer(broker, snap, nil, 0)

	md := metadata.New(map[string]string{"x-vault-pubkey": pubB64})
	ctx := metadata.NewIncomingContext(auth.WithM2MToken(context.Background(), "jwt"), md)
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"passbox/"}}

	err := srv.Watch(req, stream)
	require.Error(t, err, "resolve failure must fail-closed")
}

func TestWatch_VaultWithoutPubkeyStaysMaskedPlain(t *testing.T) {
	broker := NewBroker(nil)
	snap := &fakeSnap{entries: []nboxclient.Entry{{Key: "passbox/k", Value: "*****", Secure: true}}}
	srv := NewStreamServer(broker, snap, nil, 0)

	ctx, cancel := context.WithCancel(auth.WithM2MToken(context.Background(), "jwt"))
	stream := &mockStream{ctx: ctx}
	req := &streamv1.WatchRequest{Prefixes: []string{"passbox/"}}

	done := make(chan error, 1)
	go func() { done <- srv.Watch(req, stream) }()
	require.Eventually(t, func() bool { return len(stream.snapshotOf()) == 1 }, time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	require.Empty(t, stream.snapshotOf()[0].Extensions["encrypted"], "no pubkey => no sealing")
}

func TestWatch_EmitsKeepalivesWhenIdle(t *testing.T) {
	t.Parallel()
	broker := NewBroker(nil)
	srv := NewStreamServer(broker, nil, nil, KeepaliveInterval(20*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockStream{ctx: ctx}
	done := make(chan error, 1)
	go func() { done <- srv.Watch(&streamv1.WatchRequest{Prefixes: []string{"p/"}}, stream) }()

	require.Eventually(t, func() bool { return len(stream.snapshotOf()) >= 2 }, time.Second, 5*time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	for _, evt := range stream.snapshotOf() {
		require.Equal(t, string(event.Keepalive), evt.Type)
		require.Equal(t, "nbox", evt.Source)
		require.Empty(t, evt.Subject, "keepalive carries no subject")
		require.Empty(t, evt.Data, "keepalive carries no payload")
		require.Positive(t, evt.TimeUnixMs)
		require.NotEmpty(t, evt.Id)
	}
}

func TestWatch_ZeroIntervalDisablesKeepalive(t *testing.T) {
	t.Parallel()
	broker := NewBroker(nil)
	srv := NewStreamServer(broker, nil, nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockStream{ctx: ctx}
	done := make(chan error, 1)
	go func() { done <- srv.Watch(&streamv1.WatchRequest{}, stream) }()

	time.Sleep(100 * time.Millisecond) // enough for several 20ms ticks if the disable were broken
	cancel()
	require.NoError(t, <-done)
	require.Empty(t, stream.snapshotOf())
}

func TestWatch_KeepalivesInterleaveWithDeltas(t *testing.T) {
	t.Parallel()
	broker := NewBroker(nil)
	srv := NewStreamServer(broker, nil, nil, KeepaliveInterval(20*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockStream{ctx: ctx}
	done := make(chan error, 1)
	go func() { done <- srv.Watch(&streamv1.WatchRequest{Prefixes: []string{"p/"}}, stream) }()

	require.Eventually(t, func() bool {
		broker.Publish(&streamv1.Event{Id: "d1", Type: string(event.EntryUpserted), Subject: "p/a"})
		types := map[string]bool{}
		for _, evt := range stream.snapshotOf() {
			types[evt.Type] = true
		}
		return types[string(event.EntryUpserted)] && types[string(event.Keepalive)]
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-done)
}

func mustKey(t *testing.T) (*ecdh.PrivateKey, *ecdh.PublicKey) {
	t.Helper()
	p, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	return p, p.PublicKey()
}

// blockingSnap blocks inside Snapshot until release is closed, so a test
// can publish a delta while Watch is mid-fetch.
type blockingSnap struct {
	entries []nboxclient.Entry
	release chan struct{}
	mu      sync.Mutex
	in      bool
}

func (b *blockingSnap) entered() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.in
}

func (b *blockingSnap) Snapshot(_ context.Context, _, _ string) ([]nboxclient.Entry, error) {
	b.mu.Lock()
	b.in = true
	b.mu.Unlock()
	<-b.release
	return b.entries, nil
}

func (b *blockingSnap) Resolve(_ context.Context, _, _ string) (string, error) { return "", nil }
