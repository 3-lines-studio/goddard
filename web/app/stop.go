package app

import (
	"context"
	"sync"
)

// stops is what is running right now, by conversation: the way to cut a turn
// short. A turn is a goroutine with a context of its own — it has to be, the
// request that asked for it is gone — and this is the cancel that reaches it.
//
// It lives in the process, like the deltas of the hub, so with more than one
// instance it only reaches the turn running here. A stop that finds nothing is
// a stop that missed, and the turn goes on where it was.
type stops struct {
	mu   sync.Mutex
	byID map[string]context.CancelFunc
}

func newStops() *stops {
	return &stops{byID: map[string]context.CancelFunc{}}
}

func (s *stops) add(conversation string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[conversation] = cancel
}

func (s *stops) drop(conversation string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, conversation)
}

func (s *stops) stop(conversation string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel, ok := s.byID[conversation]
	if !ok {
		return false
	}
	cancel()
	return true
}

// Stop cuts the turn of that conversation short, if this instance is the one
// running it.
func (s *Service) Stop(conversation string) bool {
	return s.Stops.stop(conversation)
}
