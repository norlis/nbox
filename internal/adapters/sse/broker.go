package sse

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Message es la estructura que se pasa por los canales internos y se envía a los clients.
type Message struct {
	Name    string
	Payload []byte
}

// client representa a un único cliente suscrito, identificado por su ID.
type client struct {
	id      string
	channel chan Message
}

// EventBroker gestiona los clients conectados y la difusión de eventos.
type EventBroker struct {
	logger      *zap.Logger
	clients     map[string]chan Message // Clave: ClientID
	newClients  chan client
	deadClients chan client
	events      chan Message
	mu          sync.Mutex
}

// NewEventBroker ahora se integra con el ciclo de vida de fx.
func NewEventBroker(lc fx.Lifecycle, logger *zap.Logger) *EventBroker {
	broker := &EventBroker{
		logger:  logger,
		clients: make(map[string]chan Message),
		// Añadimos un búfer para evitar bloqueos en el registro/desregistro.
		newClients:  make(chan client, 10),
		deadClients: make(chan client, 10),
		events:      make(chan Message),
	}

	brokerCtx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go broker.listen(brokerCtx)
			logger.Info("Event broker started.")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping event broker...")
			cancel()
			return nil
		},
	})

	return broker
}

func (b *EventBroker) listen(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			b.logger.Info("Event broker shutting down.")
			return
		case c := <-b.newClients:
			b.mu.Lock()
			b.clients[c.id] = c.channel
			b.mu.Unlock()
			b.logger.Info("New SSE client connected", zap.String("clientId", c.id))
		case c := <-b.deadClients:
			b.mu.Lock()
			delete(b.clients, c.id)
			b.mu.Unlock()
			close(c.channel)
			b.logger.Info("SSE client disconnected", zap.String("clientId", c.id))
		case event := <-b.events:
			b.mu.Lock()
			// Iteramos sobre los clients y usamos un envío no bloqueante.
			for id, c := range b.clients {
				select {
				case c <- event:
				default:
					b.logger.Warn("Client channel is full, skipping event", zap.String("clientId", id))
				}
			}
			b.mu.Unlock()
		}
	}
}

// ServeHTTP gestiona las conexiones entrantes de SSE.
func (b *EventBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("clientId")
	if clientID == "" {
		clientID = uuid.NewString()
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	newClient := client{
		id:      clientID,
		channel: make(chan Message, 20),
	}
	b.newClients <- newClient

	defer func() {
		b.deadClients <- newClient
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-newClient.channel:
			_, _ = fmt.Fprintf(w, "event: %s\n", msg.Name)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg.Payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}
