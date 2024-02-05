package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cszczepaniak/devserve/filesystem"
	"github.com/cszczepaniak/devserve/websockets"
	"github.com/fsnotify/fsnotify"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	app := &cli.App{
		Name:  "devserve",
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
		// Default to the current directory.
		dir = "."
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	port := cCtx.Int("port")
	wsPort := cCtx.Int("ws-port")

	eg, ctx := errgroup.WithContext(cCtx.Context)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// TODO call Add if we notice a creation event for a subdirectory in this dir.
	err = watcher.Add(absDir)
	if err != nil {
		return err
	}
	defer watcher.Close()
	log.Printf("listening for changes in directory %s\n", absDir)

	// Serve the websocket server which tells the browser when to reload the page.
	eg.Go(func() error {
		wsServer := websockets.NewServer(
			watcher.Events,
			watcher.Errors,
			port,
		)
		wsServer.Start(ctx)

		return http.ListenAndServe(
			":"+strconv.Itoa(wsPort),
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wsServer.ServeHTTP(w, r.WithContext(ctx))
			}),
		)
	})
	log.Printf("serving websocket on port %d\n", wsPort)

	// Serve the files in the given directory.
	eg.Go(func() error {
		fileServer := http.FileServer(
			filesystem.New(http.Dir(dir), wsPort),
		)

		return http.ListenAndServe(
			":"+strconv.Itoa(port),
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fileServer.ServeHTTP(w, r)
				w.Header().Set("Clear-Site-Data", "cache")
			}),
		)
	})
	log.Printf("serving directory %s on port %d\n", absDir, port)

	return eg.Wait()
}
