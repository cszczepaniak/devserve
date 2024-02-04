package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/cszczepaniak/go-devserve/filesystem"
	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	app := &cli.App{
		Name:  "go-devserve",
		Usage: "A development file server with live-reloading.",
		Args:  true,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "port",
				Usage: "The port used to serve the given directory.",
				Value: 3000,
			},
			&cli.IntFlag{
				Name:  "ws-port",
				Usage: "The port used to serve the websocket that notifies the browser when to live-reload the files in the given directory.",
				Value: 8090,
			},
		},
		Action: runServer,
	}

	err := app.RunContext(context.Background(), os.Args)
	if err != nil {
		panic(err)
	}
}

func runServer(cCtx *cli.Context) error {
	dir := cCtx.Args().First()
	if dir == "" {
		return errors.New("must provide a directory")
	}

	port := cCtx.Int("port")
	wsPort := cCtx.Int("ws-port")

	eg, ctx := errgroup.WithContext(cCtx.Context)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// TODO call Add if we notice a creation event for a subdirectory in this dir.
	err = watcher.Add(dir)
	if err != nil {
		return err
	}

	wsServer := websocketServer{
		ctx:    ctx,
		events: watcher.Events,
	}

	eg.Go(func() error {
		return http.ListenAndServe(":"+strconv.Itoa(wsPort), wsServer)
	})

	fileServer := http.FileServer(
		filesystem.New(http.Dir(dir), wsPort),
	)
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
		w.Header().Set("Clear-Site-Data", "cache")
	}))

	eg.Go(func() error {
		return http.ListenAndServe(":"+strconv.Itoa(port), nil)
	})

	return eg.Wait()
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
