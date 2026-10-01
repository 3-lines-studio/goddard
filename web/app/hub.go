package app

import "sync"

// hub is what a turn says while it is still running: the deltas of a message
// being written, which are worth watching and not worth keeping. The log holds
// what the message ended up being; this holds it in flight.
//
// It lives in the process, so with more than one instance the deltas only reach
// the clients connected to the one running the turn. The message itself always
// arrives, because the log is in the database: this is a faster path, not the
// only one. The day there are several instances, the way out is `LISTEN`.
type hub struct {
	mu   sync.Mutex
	byID map[string]map[chan []byte]struct{}
}

func newHub() *hub {
	return &hub{byID: map[string]map[chan []byte]struct{}{}}
}

// listen is the stream of one conversation while somebody is watching it.
func (h *hub) listen(conversation string) (chan []byte, func()) {
	channel := make(chan []byte, 64)
	h.mu.Lock()
	if h.byID[conversation] == nil {
		h.byID[conversation] = map[chan []byte]struct{}{}
	}
	h.byID[conversation][channel] = struct{}{}
	h.mu.Unlock()
	return channel, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.byID[conversation], channel)
		if len(h.byID[conversation]) == 0 {
			delete(h.byID, conversation)
		}
	}
}

// tell hands the line to whoever is watching. A client that is not reading
// loses it: it is a delta, and the log has the message.
func (h *hub) tell(conversation string, body []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for channel := range h.byID[conversation] {
		select {
		case channel <- body:
		default:
		}
	}
}
