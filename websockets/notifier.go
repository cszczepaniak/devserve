package websockets

import (
	"errors"
	"log"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type notifier struct {
	mu        sync.Mutex
	notifiers []func(fsnotify.Event) error
}

func newNotifier() *notifier {
	return &notifier{}
}

func (n *notifier) add(fn func(fsnotify.Event) error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.notifiers = append(n.notifiers, fn)
}

func (n *notifier) notify(event fsnotify.Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	log.Printf("notifying %d connections...\n", len(n.notifiers))
	var errs []error
	for _, fn := range n.notifiers {
		err := fn(event)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	return nil
}
