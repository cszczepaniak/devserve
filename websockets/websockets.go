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

	notifier *notifier
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
		notifier:        newNotifier(),
	}
}

func (s websocketServer) Start(ctx context.Context) {
	go s.listenToEvents(ctx)
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

	s.notifier.add(func(e fsnotify.Event) error {
		return s.handleEvent(c, e)
	})

	<-ctx.Done()
}

func (s websocketServer) listenToEvents(ctx context.Context) {
	log.Println("websocket server listening for file change events...")
	for {
		select {
		case <-ctx.Done():
			log.Println("websocket event listener exiting...")
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
			err := s.notifier.notify(event)
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
