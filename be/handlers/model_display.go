package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"example.com/mishis4x/api"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// modelDisplayPingInterval/modelDisplayPongWait/modelDisplayWriteWait bound
// the WebSocket keepalive in ServeModelDisplayWS - see its own doc comment.
// pongWait is comfortably longer than pingInterval so one missed pong (a
// slow/busy client, not a dead one) doesn't immediately kill the
// connection - it only actually times out after missing a full ping cycle,
// not just one slightly-late reply.
const (
	modelDisplayPingInterval = 30 * time.Second
	modelDisplayPongWait     = 60 * time.Second
	modelDisplayWriteWait    = 10 * time.Second
)

// modelDisplayUpgrader has no real CORS story to enforce (CheckOrigin
// returning true unconditionally) - this app doesn't serve the frontend
// from a different origin than the API at all (see CLAUDE.md's "ships as
// one binary" architecture), and the upgrade request itself already goes
// through the same AuthMiddleware/modelOnlyMiddleware chain every other
// /api/models/... route does - Origin-checking would be a second, redundant
// guard against exactly what session auth already covers.
var modelDisplayUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ModelDisplay holds the shared "what should the physically-mounted
// display be showing right now" state for the model viewer's remote-
// control feature - a controller (anyone browsing /models/{charCode})
// writes to it via Set/SetFlip/SetTransform/IncrementTrigger, and a
// display (a phone stuck inside a physical Pepper's Ghost/acrylic rig,
// with no practical way to interact with it directly) holds open a
// WebSocket (ServeModelDisplayWS) that gets pushed the new state the
// moment any of those writers changes something. Deliberately unpersisted,
// in-memory only, same single-instance-is-fine precedent as
// matchmaking.Lobby: this is ephemeral "what's on screen right now"
// state, not data worth surviving a restart, and this app already only
// ever runs as one instance - which also means a plain in-process
// subscriber list (no Redis-style pub/sub) is enough to fan updates out,
// since there's only ever one process for "the other display" to connect
// to in the first place.
//
// The mutex guards the whole struct, not each field/the subscriber set
// separately - a display should never see e.g. a new CharCode paired with
// a stale Flip value left over from before a concurrent write finished,
// and a subscribe() racing a broadcast() must land cleanly on one side of
// it or the other, not observe a half-updated subscriber map.
//
// Four separate writers (Set/SetFlip/SetTransform/IncrementTrigger), not
// one PUT of the whole struct: each covers a distinct, independently-
// triggered controller action (pick a character, live-tweak orientation,
// live-tweak pan/zoom, fire a touch reaction), and a caller for one was
// never guaranteed to know or safely resend the others' current values -
// see Set's own doc comment for the concrete bug this already caused
// once with Trigger.
type ModelDisplay struct {
	mu          sync.Mutex
	state       api.ModelDisplayState
	subscribers map[chan api.ModelDisplayState]struct{}
}

// Get returns the current shared display state - the zero-value
// ModelDisplayState (empty CharCode) is the ordinary state for a server
// that's never had SetModelDisplay called yet this run, not an error.
// Kept around for GET /api/models/display's manual/test-inspection use -
// the real display no longer polls this at all (see ServeModelDisplayWS),
// so this is no longer anything's actual live path.
func (d *ModelDisplay) Get() api.ModelDisplayState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

// Set overwrites CharCode and Flip together - "Set as display"/Broadcast
// picking a new character and its starting orientation in one action.
// Deliberately not Trigger too: a caller here only ever knows/sends
// char_code+flip, and once Trigger existed, this same method overwriting
// the whole struct would have reset it back to zero on every single write
// - which the display would read as a real change and fire a spurious
// touch reaction. Trigger only ever moves via IncrementTrigger, Flip-only
// live-tweaks only ever move via SetFlip, pan/zoom only ever moves via
// SetTransform - each field has exactly one writer that owns it. Pushes
// the new state to every connected display immediately after (see
// broadcast) - this used to only take effect on a display's next poll,
// up to 1.5s later; a real connection means there's no poll to wait for.
func (d *ModelDisplay) Set(charCode, flip string) {
	d.mu.Lock()
	d.state.CharCode = charCode
	d.state.Flip = flip
	d.mu.Unlock()
	d.broadcast()
}

// SetFlip overwrites just Flip, leaving CharCode/Trigger/pan-zoom
// untouched - what a controller's Flip toggle live-pushes to an already-
// connected display. Its own method (not routed through Set) specifically
// so live-tweaking orientation never has to know or resend whichever
// character actually happens to be live on the display right now.
func (d *ModelDisplay) SetFlip(flip string) {
	d.mu.Lock()
	d.state.Flip = flip
	d.mu.Unlock()
	d.broadcast()
}

// SetTransform overwrites just the pan/zoom correction, leaving
// CharCode/Flip/Trigger untouched - what a controller's arrow/zoom nudge
// buttons live-push to an already-connected display. Its own method for
// the same reason SetFlip is its own method rather than folded into Set.
func (d *ModelDisplay) SetTransform(offsetX, offsetY int, zoom float64) {
	d.mu.Lock()
	d.state.OffsetX = offsetX
	d.state.OffsetY = offsetY
	d.state.Zoom = zoom
	d.mu.Unlock()
	d.broadcast()
}

// IncrementTrigger bumps the shared trigger counter - see
// api.ModelDisplayState.Trigger's own doc comment for why a plain
// increment rather than a boolean/timestamp.
func (d *ModelDisplay) IncrementTrigger() {
	d.mu.Lock()
	d.state.Trigger++
	d.mu.Unlock()
	d.broadcast()
}

// Connected reports whether at least one display is actually holding a
// live WebSocket connection right now - what a controller's Trigger
// touch/pan-zoom nudges are enabled/disabled by, so they don't offer to
// act on a display that isn't there. Replaced an earlier polling-
// staleness heuristic (a display was "connected" if it had GET-polled
// within the last few seconds, cleared lazily whenever the next status
// check noticed it'd gone quiet) - a real connection count is exact where
// a timestamp heuristic was only ever an approximation, and comes for
// free once the display is a genuine long-lived connection instead of a
// poll loop.
func (d *ModelDisplay) Connected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.subscribers) > 0
}

// subscribe registers a new display connection and returns a channel that
// broadcast (below) pushes the current state to on every change - pre-
// seeded with a snapshot of the state right now, under the same lock, so
// a newly-connected display doesn't have to wait for the *next* change to
// see whatever's already live. Buffered to exactly 1: broadcast only ever
// cares about delivering the latest value, never a backlog (see its own
// doc comment), so there's nothing a bigger buffer would help with.
func (d *ModelDisplay) subscribe() chan api.ModelDisplayState {
	ch := make(chan api.ModelDisplayState, 1)
	d.mu.Lock()
	if d.subscribers == nil {
		d.subscribers = make(map[chan api.ModelDisplayState]struct{})
	}
	d.subscribers[ch] = struct{}{}
	ch <- d.state
	d.mu.Unlock()
	return ch
}

// unsubscribe removes ch - called exactly once per subscribe(), whenever
// that connection ends for any reason (a real close, a network drop, a
// missed ping). Also where the actual state-clearing happens once the
// last display disconnects: nothing else would ever notice a departed
// display on its own, so this is the one moment that's knowable at all -
// same reasoning the old poll-based Connected() had for clearing lazily,
// just event-driven now that there's a real disconnect to hang it on
// instead of an inferred timeout.
func (d *ModelDisplay) unsubscribe(ch chan api.ModelDisplayState) {
	d.mu.Lock()
	delete(d.subscribers, ch)
	if len(d.subscribers) == 0 {
		d.state = api.ModelDisplayState{}
	}
	d.mu.Unlock()
}

// broadcast pushes the current state to every subscriber - called by
// every writer above right after it changes something. Non-blocking,
// latest-value-wins per subscriber (drains a still-pending value before
// pushing the new one) rather than an unbounded queue: a subscriber
// that's fallen behind (a slow network, a backgrounded tab) should catch
// up to wherever things are *now* whenever it does read again, not replay
// every intermediate state a rapid pan/zoom nudge burst produced along
// the way.
func (d *ModelDisplay) broadcast() {
	d.mu.Lock()
	state := d.state
	chans := make([]chan api.ModelDisplayState, 0, len(d.subscribers))
	for ch := range d.subscribers {
		chans = append(chans, ch)
	}
	d.mu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- state:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- state:
			default:
			}
		}
	}
}

// ServeModelDisplayWS is the real display's live connection - it replaces
// the old GET /api/models/display poll loop entirely (ModelDisplay.tsx no
// longer polls at all) so a controller's change reaches the display the
// moment it happens instead of waiting up to one poll interval - confirmed
// live that this was the single biggest source of felt lag adjusting
// pan/zoom against the real physical rig.
//
// One dedicated goroutine (this handler's own, below) owns every write to
// conn - both state pushes read off the send channel and this handler's
// own ping frames - gorilla/websocket's Conn only supports one concurrent
// writer, and funneling every write through one loop keeps that invariant
// easy to see rather than threading a mutex through multiple write sites.
// A second, separate goroutine only *reads* - the display never actually
// sends anything meaningful, but something still has to keep calling
// ReadMessage so a real close/error is noticed and so SetPongHandler ever
// gets a chance to fire (a Conn doesn't process control frames unless
// something's actively reading). Reading and writing the same Conn
// concurrently is fine - gorilla/websocket only forbids concurrent
// *writers* or concurrent *readers* with each other, not one of each.
func (d *Data) ServeModelDisplayWS(w http.ResponseWriter, r *http.Request) {
	conn, err := modelDisplayUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("error upgrading model display websocket")
		return
	}
	defer func() { _ = conn.Close() }()

	send := d.ModelDisplay.subscribe()
	defer d.ModelDisplay.unsubscribe(send)

	// Closed once the read loop below notices the connection is gone (a
	// real close, a network drop, or a missed pong past
	// modelDisplayPongWait) - the only signal the write loop needs to
	// know it's time to stop.
	closed := make(chan struct{})
	_ = conn.SetReadDeadline(time.Now().Add(modelDisplayPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(modelDisplayPongWait))
	})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(modelDisplayPingInterval)
	defer ticker.Stop()

	for {
		select {
		case state := <-send:
			_ = conn.SetWriteDeadline(time.Now().Add(modelDisplayWriteWait))
			if err := conn.WriteJSON(state); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(modelDisplayWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-closed:
			return
		}
	}
}

// GetModelDisplay returns the current shared display state - see
// ModelDisplay.Get's own doc comment for why this is no longer anything's
// real live path, kept for manual/test inspection.
func (d *Data) GetModelDisplay(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, d.ModelDisplay.Get())
}

// GetModelDisplayStatus reports whether a display is currently considered
// connected (see ModelDisplay.Connected) - a separate endpoint from
// GetModelDisplay specifically so a controller polling this to drive its
// own UI is asking a cheap, side-effect-free question rather than
// touching anything.
func (d *Data) GetModelDisplayStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.ModelDisplayStatus{Connected: d.ModelDisplay.Connected()})
}

// TriggerModelDisplay bumps the shared display's trigger counter (see
// api.ModelDisplayState.Trigger) - a separate endpoint from
// SetModelDisplay/SetModelDisplayFlip/SetModelDisplayTransform so
// triggering a touch reaction never has to also resend anything else.
func (d *Data) TriggerModelDisplay(w http.ResponseWriter, r *http.Request) {
	d.ModelDisplay.IncrementTrigger()
	w.WriteHeader(http.StatusOK)
}

// SetModelDisplay picks which character the display shows, and its
// starting orientation in the same action ("Set as display"/Broadcast) -
// see ModelDisplay.Set's own doc comment for why this only ever touches
// those two fields. An empty CharCode is accepted as a real, deliberate
// "nothing selected" state (e.g. clearing the display before a show
// starts), not an error - only a non-empty CharCode gets checked against
// character_models, so a typo'd code fails loudly here rather than the
// display silently getting stuck trying to load something that was never
// imported.
func (d *Data) SetModelDisplay(w http.ResponseWriter, r *http.Request) {
	var body api.SetModelDisplayCharCodeInput
	if !decodeJSONBody(w, r, &body) {
		return
	}

	if body.CharCode != "" {
		ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
		defer cancel()

		// CharacterModelExists, not GetCharacterModel: this is only
		// validating the code, not serving the model, so there's no
		// reason to pull the (potentially multi-MB) skeleton/atlas/
		// texture blobs over the wire just to throw them away - that
		// wasted transfer was itself eating into this same 5s budget
		// for no benefit.
		exists, err := d.P.CharacterModelExists(ctx, body.CharCode)
		if err != nil {
			log.Error().Err(err).Str("charCode", body.CharCode).Msg("error checking character model for display")
			writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		if !exists {
			writeJSONError(w, http.StatusNotFound, "Model not found.")
			return
		}
	}

	d.ModelDisplay.Set(body.CharCode, body.Flip)
	w.WriteHeader(http.StatusOK)
}

// SetModelDisplayFlip live-adjusts just the display's current
// orientation - see ModelDisplay.SetFlip's own doc comment for why this
// is a separate endpoint from SetModelDisplay's character-picking PUT.
// No validation needed: unlike a char_code, an unrecognized flip value
// isn't a lookup into anything - ModelDisplay.tsx/ModelViewer.tsx's own
// FLIP_TRANSFORMS lookup already tolerates one by falling back to no
// transform, the same tolerance this endpoint just passes through.
func (d *Data) SetModelDisplayFlip(w http.ResponseWriter, r *http.Request) {
	var body api.SetModelDisplayFlipInput
	if !decodeJSONBody(w, r, &body) {
		return
	}
	d.ModelDisplay.SetFlip(body.Flip)
	w.WriteHeader(http.StatusOK)
}

// SetModelDisplayTransform live-adjusts just the display's pan/zoom
// correction - see ModelDisplay.SetTransform's own doc comment for why
// this is a separate endpoint from both SetModelDisplay and
// SetModelDisplayFlip. No validation needed: an offset is just a screen-
// pixel count and a zoom of 0 (the zero value, meaning "never touched")
// is already the frontend's own signal to render at 1x - see
// api.ModelDisplayState.Zoom's own doc comment.
func (d *Data) SetModelDisplayTransform(w http.ResponseWriter, r *http.Request) {
	var body api.SetModelDisplayTransformInput
	if !decodeJSONBody(w, r, &body) {
		return
	}
	d.ModelDisplay.SetTransform(body.OffsetX, body.OffsetY, body.Zoom)
	w.WriteHeader(http.StatusOK)
}
