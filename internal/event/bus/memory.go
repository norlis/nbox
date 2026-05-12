package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"nbox/internal/event"
)

const (
	subscriberBuffer = 20
	keepAliveEvery   = 15 * time.Second
	clientReconnect  = "retry: 5000\n\n"
)

// Message es la unidad que se distribute a cada cliente SSE.
type Message struct {
	ID      string
	Name    string
	Payload []byte
}

// Memory es un bus pub/sub en memoria que publica eventos a clients SSE.
// Implementa event.Publisher y http.Handler (para suscripciones SSE).
//
// Los suscriptores se trackean en un mapa keyed by client ID, protegido por
// un RWMutex para que el fan-out (Publish) no se serialice contra sí mismo,
// solo contra subscribe/unsubscribe.
type Memory struct {
	logger      *zap.Logger
	mu          sync.RWMutex
	subscribers map[string]chan Message
	closed      bool
}

// NewMemory crea un bus en memoria. Registra hooks de lifecycle en fx para
// cerrar limpiamente al apagar la app.
func NewMemory(lc fx.Lifecycle, logger *zap.Logger) *Memory {
	b := &Memory{
		logger:      logger,
		subscribers: make(map[string]chan Message),
	}

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			logger.Info("Event bus started")
			return nil
		},
		OnStop: func(_ context.Context) error {
			b.shutdown()
			logger.Info("Event bus stopped")
			return nil
		},
	})

	return b
}

// Publish implementa event.Publisher.
func (b *Memory) Publish(_ context.Context, e event.Event[json.RawMessage]) error {
	payloadBytes, err := json.Marshal(e)
	if err != nil {
		b.logger.Error("ErrBusEventEncode", zap.Error(err))
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	b.fanOut(Message{
		Name:    string(e.Type),
		Payload: payloadBytes,
	})
	return nil
}

// Subscribe registra un cliente. Si id está vacío se genera un UUID v7.
// Si id ya está tomado, se cierra el canal anterior y se instala uno nuevo.
func (b *Memory) Subscribe(id string) (string, <-chan Message) {
	if id == "" {
		if u, err := uuid.NewV7(); err == nil {
			id = u.String()
		} else {
			id = uuid.NewString()
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan Message)
		close(ch)
		return id, ch
	}

	if old, ok := b.subscribers[id]; ok {
		close(old)
		b.logger.Debug("evicted previous subscriber with same id", zap.String("clientId", id))
	}

	ch := make(chan Message, subscriberBuffer)
	b.subscribers[id] = ch
	return id, ch
}

// Unsubscribe elimina al cliente y cierra su canal. Idempotente.
func (b *Memory) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

// fanOut distribute el mensaje a cada suscriptor activo. Suscriptores lentos
// (buffer lleno) descartan el evento en lugar de bloquear al publisher.
func (b *Memory) fanOut(msg Message) {
	if msg.ID == "" {
		if u, err := uuid.NewV7(); err == nil {
			msg.ID = u.String()
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			b.logger.Warn("subscriber buffer full, dropping event",
				zap.String("clientId", id),
				zap.String("event", msg.Name),
			)
		}
	}
}

func (b *Memory) shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
}

// ServeHTTP transmite eventos a un único cliente SSE.
func (b *Memory) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Long-lived stream: clear any server-wide WriteTimeout for this request.
	_ = rc.SetWriteDeadline(time.Time{})

	if err := rc.Flush(); err != nil {
		b.logger.Error("streaming unsupported by ResponseWriter", zap.Error(err))
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	id, ch := b.Subscribe(r.URL.Query().Get("clientId"))
	defer b.Unsubscribe(id)

	b.logger.Debug("SSE client connected",
		zap.String("clientId", id),
		zap.String("remoteAddr", r.RemoteAddr),
	)

	if _, err := fmt.Fprint(w, clientReconnect); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(keepAliveEvery)
	defer ticker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			b.logger.Debug("SSE client disconnected", zap.String("clientId", id))
			return

		case msg, ok := <-ch:
			if !ok {
				b.logger.Debug("subscriber channel closed, terminating SSE stream",
					zap.String("clientId", id),
				)
				return
			}
			if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", msg.ID, msg.Name, msg.Payload); err != nil {
				b.logger.Debug("failed to write event payload, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}
			if err := rc.Flush(); err != nil {
				b.logger.Debug("flush failed after event, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}

		case <-ticker.C:
			if _, err := fmt.Fprint(w, ":ping\n\n"); err != nil {
				b.logger.Debug("failed to write keep-alive ping, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}
			if err := rc.Flush(); err != nil {
				b.logger.Debug("flush failed after ping, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}
		}
	}
}
