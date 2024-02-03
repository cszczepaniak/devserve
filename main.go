package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"strconv"

	"github.com/cszczepaniak/go-devserve/filesystem"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	var applicationPort int
	var websocketPort int
	var dir string

	flag.IntVar(&applicationPort, "app-port", 3000, "The port to serve the given directory.")
	flag.IntVar(&websocketPort, "ws-port", 8090, "The port to serve the websocket used to live-reload the files in the given directory.")
	flag.StringVar(&dir, "dir", ".", "The directory to serve. Defaults to the current directory.")

	flag.Parse()

	eg, ctx := errgroup.WithContext(context.Background())

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	// TODO call Add if we notice a creation event for a subdirectory in this dir.
	err = watcher.Add(dir)
	if err != nil {
		panic(err)
	}

	wsServer := websocketServer{
		ctx:    ctx,
		events: watcher.Events,
	}

	eg.Go(func() error {
		return http.ListenAndServe(":"+strconv.Itoa(websocketPort), wsServer)
	})

	fileServer := http.FileServer(
		filesystem.New(http.Dir(dir), websocketPort),
	)
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
		w.Header().Set("Clear-Site-Data", "cache")
	}))

	eg.Go(func() error {
		return http.ListenAndServe(":"+strconv.Itoa(applicationPort), nil)
	})

	err = eg.Wait()
	if err != nil {
		panic(err)
	}
}

type websocketServer struct {
	ctx    context.Context
	events <-chan fsnotify.Event
}

func (s websocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:3000"},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.CloseNow()

	ctx := c.CloseRead(s.ctx)

	for {
		select {
		case <-ctx.Done():
			return
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

	log.Println("handling write event for", event.Name)
	return wsjson.Write(context.Background(), c, "file change")
}
