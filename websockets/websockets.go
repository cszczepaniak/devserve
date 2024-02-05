package websockets

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/fsnotify/fsnotify"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type websocketServer struct {
	events <-chan fsnotify.Event
	errors <-chan error

	applicationPort int

	mu sync.Mutex

	// connections is a map from connection to a channel which will be closed when the connection is
	// closed.
	connections map[*websocket.Conn]<-chan struct{}
}

func NewServer(
	events <-chan fsnotify.Event,
	errors <-chan error,
	applicationPort int,
) websocketServer {
	return websocketServer{
		events:          events,
		errors:          errors,
		applicationPort: applicationPort,
		connections:     make(map[*websocket.Conn]<-chan struct{}),
	}
}

func (s *websocketServer) Start(ctx context.Context) {
	go s.listenToEvents(ctx)
}

func (s *websocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{fmt.Sprintf("localhost:%d", s.applicationPort)},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("websocket connected")

	// We won't be reading from this channel. The ctx gets closed when the client closes the
	// connection.
	ctx := c.CloseRead(r.Context())

	select {
	case <-ctx.Done():
		// If the connection has already been closed, skip adding it entirely.
		return
	default:
		s.addConnection(ctx, c)
	}

	go func() {
		<-ctx.Done()

		// Once the client is closed, we should remove its connection so we no longer try to notify
		// it.
		s.removeConnection(c)
	}()
}

func (s *websocketServer) listenToEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-s.errors:
			if !ok {
				return
			}
			if err != nil {
				log.Println("error:", err)
			}
		case event, ok := <-s.events:
			if !ok {
				return
			}

			err := s.notifyConnections(event)
			if err != nil {
				log.Println("error:", err)
			}
		}
	}
}

func (s *websocketServer) notifyConnections(event fsnotify.Event) error {
	if !event.Op.Has(fsnotify.Write) {
		return nil
	}
	log.Printf("file changed: %s\n", event.Name)

	s.mu.Lock()
	defer s.mu.Unlock()

	i := 0
	errs := make([]error, len(s.connections))
	for c, done := range s.connections {
		select {
		case <-done:
			// If this connection is closed, our cleanup goroutine will remove it. We shouldn't try
			// to write to it.
			continue
		default:
		}

		err := wsjson.Write(context.Background(), c, "file change")
		if err != nil {
			errs[i] = err
		}
		i++
	}

	return errors.Join(errs...)
}

func (s *websocketServer) addConnection(ctx context.Context, c *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connections[c] = ctx.Done()
}

func (s *websocketServer) removeConnection(c *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.connections, c)
}
