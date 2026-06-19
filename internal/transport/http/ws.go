package http

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// wsHub keeps the set of connected dashboard browsers and pushes a data-revision
// message whenever the backend data changes — giving instant, no-refresh updates
// over a WebSocket. The browser re-fetches its data on each push.
//
// The revision is bumped by the bumpOnWrite middleware after every successful
// mutating request (POST/PUT/PATCH/DELETE), so any write — by any user — fans
// out to every open dashboard within one round-trip.
type wsHub struct {
	rev   int64 // data revision, bumped on every successful write
	mu    sync.Mutex
	conns map[*websocket.Conn]bool
}

func newWSHub() *wsHub { return &wsHub{conns: map[*websocket.Conn]bool{}} }

// revision returns the current data revision.
func (h *wsHub) revision() int64 { return atomic.LoadInt64(&h.rev) }

// bump increments the revision and broadcasts it to all connected clients.
func (h *wsHub) bump() { h.broadcast(atomic.AddInt64(&h.rev, 1)) }

// broadcast writes rev to every connection. The hub serialises writes with its
// mutex (gorilla conns allow a single concurrent writer).
func (h *wsHub) broadcast(rev int64) {
	msg := map[string]int64{"rev": rev}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteJSON(msg); err != nil {
			delete(h.conns, c)
			_ = c.Close()
		}
	}
}

func (h *wsHub) add(c *websocket.Conn) {
	h.mu.Lock()
	h.conns[c] = true
	h.mu.Unlock()
}

func (h *wsHub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
	_ = c.Close()
}

func (h *wsHub) sendTo(c *websocket.Conn, rev int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = c.WriteJSON(map[string]int64{"rev": rev})
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true }, // same-trust dev/LAN setup
}

// ws upgrades the request to a WebSocket. Browsers cannot send the Authorization
// header on a WS handshake, so the bearer token is passed as a query parameter.
func (h *Handler) ws(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.resolveUser(r.URL.Query().Get("token")); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	c, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.hub.add(c)
	h.hub.sendTo(c, h.hub.revision()) // sync immediately on connect
	// Read loop: we don't expect client messages — it only detects disconnect.
	go func() {
		defer h.hub.remove(c)
		c.SetReadLimit(512)
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
