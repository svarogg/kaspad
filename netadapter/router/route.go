package router

import (
	"sync"
	"time"

	"github.com/kaspanet/kaspad/wire"
	"github.com/pkg/errors"
)

const (
	maxMessages = 100
)

// ErrTimeout signifies that one of the router functions had a timeout.
var ErrTimeout = errors.New("timeout expired")

// onCapacityReachedHandler is a function that is to be
// called when a route reaches capacity.
type onCapacityReachedHandler func()

// Route represents an incoming or outgoing Router route
type Route struct {
	channel   chan wire.Message
	closed    bool
	closeLock sync.Mutex

	onCapacityReachedHandler onCapacityReachedHandler
}

// NewRoute create a new Route
func NewRoute() *Route {
	return &Route{
		channel: make(chan wire.Message, maxMessages),
		closed:  false,
	}
}

func (r *Route) getLock() {
	r.closeLock.Lock()
}

// Enqueue enqueues a message to the Route
func (r *Route) Enqueue(message wire.Message) (isOpen bool) {
	r.getLock()
	defer r.closeLock.Unlock()

	if r.closed {
		return false
	}
	if len(r.channel) == maxMessages {
		r.onCapacityReachedHandler()
	}
	r.channel <- message
	return true
}

// Dequeue dequeues a message from the Route
func (r *Route) Dequeue() (message wire.Message, isOpen bool) {
	return <-r.channel, !r.closed
}

// EnqueueWithTimeout attempts to enqueue a message to the Route
// and returns an error if the given timeout expires first.
func (r *Route) EnqueueWithTimeout(message wire.Message, timeout time.Duration) (isOpen bool, err error) {
	r.getLock()
	defer r.closeLock.Unlock()

	if r.closed {
		return false, nil
	}
	if len(r.channel) == maxMessages {
		r.onCapacityReachedHandler()
	}
	select {
	case <-time.After(timeout):
		return false, errors.Wrapf(ErrTimeout, "got timeout after %s", timeout)
	case r.channel <- message:
		return true, nil
	}
}

// DequeueWithTimeout attempts to dequeue a message from the Route
// and returns an error if the given timeout expires first.
func (r *Route) DequeueWithTimeout(timeout time.Duration) (message wire.Message, isOpen bool, err error) {
	select {
	case <-time.After(timeout):
		return nil, false, errors.Wrapf(ErrTimeout, "got timeout after %s", timeout)
	case message := <-r.channel:
		return message, !r.closed, nil
	}
}

func (r *Route) setOnCapacityReachedHandler(onCapacityReachedHandler onCapacityReachedHandler) {
	r.onCapacityReachedHandler = onCapacityReachedHandler
}

// Close closes this route
func (r *Route) Close() error {
	r.getLock()
	defer r.closeLock.Unlock()

	r.closed = true
	close(r.channel)
	return nil
}
