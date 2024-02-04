package websockets

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/fsnotify/fsnotify"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type websocketServer struct {
	events <-chan fsnotify.Event
	errors <-chan error

	applicationPort int
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
	}
}

func (s websocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{fmt.Sprintf("localhost:%d", s.applicationPort)},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.CloseNow()

	log.Println("websocket connected")

	ctx := c.CloseRead(r.Context())

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
			err := s.handleEvent(c, event)
			if err != nil {
				log.Println("error:", err)
			}
		}
	}
}

func (s websocketServer) handleEvent(c *websocket.Conn, event fsnotify.Event) error {
	if !event.Op.Has(fsnotify.Write) {
		return nil
	}

	log.Println("file changed:", event.Name)
	return wsjson.Write(context.Background(), c, "file change")
}
