package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"example.com/mishis4x/api"
	"github.com/rs/zerolog/log"
)

// displayStaleAfter bounds how long the shared display state is kept
// around with no confirmed live display polling it - see
// ModelDisplay.Connected's own doc comment. fe/src/components/
// ModelDisplay.tsx polls every 1.5s (POLL_INTERVAL_MS), so this is
// "about 3 missed polls" with a little slack for ordinary network
// jitter, not a hair-trigger on one slightly-late request.
//
// A var, not a const, solely so tests can shrink it (see
// setDisplayStaleAfterForTest in model_display_test.go) rather than
// actually sleeping 5+ real seconds per staleness test - never
// reassigned outside tests.
var displayStaleAfter = 5 * time.Second

// ModelDisplay holds the shared "what should the physically-mounted
// display be showing right now" state for the model viewer's remote-
// control feature - a controller (anyone browsing /models/{charCode})
// writes to it via Set, and a display (a phone stuck inside a physical
// Pepper's Ghost/acrylic rig, with no practical way to interact with it
// directly - see ModelViewer.tsx's own ?flip=/?pepper= for the same
// reasoning) polls it via Get. Deliberately unpersisted, in-memory only,
// same single-instance-is-fine precedent as matchmaking.Lobby: this is
// ephemeral "what's on screen right now" state, not data worth
// surviving a restart, and this app already only ever runs as one
// instance.
//
// The mutex guards the whole struct, not each field separately - a
// display mid-poll should never see e.g. a new CharCode paired with a
// stale Pepper value left over from before a concurrent write finished.
type ModelDisplay struct {
	mu    sync.Mutex
	state api.ModelDisplayState
	// lastPolledAt is the last time Get was called - Get is what the
	// real display calls every poll, so this doubles as that display's
	// own heartbeat with no separate endpoint needed for it.
	lastPolledAt time.Time
}

// Get returns the current shared display state, and records this call
// as proof the display is still alive (see lastPolledAt/Connected) -
// only ever called by the actual display polling loop, never by a
// controller just checking status (that's Connected, which deliberately
// doesn't touch this timestamp - see its own doc comment for why).
func (d *ModelDisplay) Get() api.ModelDisplayState {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastPolledAt = time.Now()
	return d.state
}

// Set overwrites CharCode/Flip/Pepper - deliberately not the whole
// struct, so a controller sending "Set as display" (which only ever
// knows/sends those three fields) can never reset Trigger back to
// whatever zero value happened to be in that request body, which would
// otherwise register as a real change and fire a spurious touch
// reaction on the display's very next poll. Trigger only ever moves via
// IncrementTrigger below.
func (d *ModelDisplay) Set(state api.ModelDisplayState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.CharCode = state.CharCode
	d.state.Flip = state.Flip
	d.state.Pepper = state.Pepper
}

// IncrementTrigger bumps the shared trigger counter - see
// api.ModelDisplayState.Trigger's own doc comment for why a plain
// increment rather than a boolean/timestamp.
func (d *ModelDisplay) IncrementTrigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.Trigger++
}

// Connected reports whether a real display has polled Get within
// displayStaleAfter - what a controller's "Set as display" button is
// enabled/disabled by, so it doesn't send to a display that's been
// closed/gone for a while. Deliberately doesn't call Get or otherwise
// touch lastPolledAt itself: if this bumped the same timestamp Get
// does, a controller repeatedly checking status would look identical to
// a live display, defeating the whole point.
//
// Also where the actual clearing happens, not a separate background
// job: if the display's gone, nothing else will ever notice on its own
// (the display can't tell anyone it stopped polling) - the next time an
// actual human checks status from the controller is the first
// opportunity to notice and clear the stale state, so it happens right
// here rather than needing a ticker/goroutine for what's a rare,
// human-paced event anyway.
func (d *ModelDisplay) Connected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastPolledAt.IsZero() {
		return false
	}
	if time.Since(d.lastPolledAt) > displayStaleAfter {
		d.state = api.ModelDisplayState{}
		return false
	}
	return true
}

// GetModelDisplay returns the current shared display state - the
// zero-value ModelDisplayState (empty CharCode) is the ordinary state
// for a server that's never had SetModelDisplay called yet this run,
// not an error.
func (d *Data) GetModelDisplay(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, d.ModelDisplay.Get())
}

// GetModelDisplayStatus reports whether a display is currently
// considered connected (see ModelDisplay.Connected) - a separate
// endpoint from GetModelDisplay specifically so a controller polling
// this to drive its own UI never itself counts as "the display is
// alive".
func (d *Data) GetModelDisplayStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.ModelDisplayStatus{Connected: d.ModelDisplay.Connected()})
}

// TriggerModelDisplay bumps the shared display's trigger counter (see
// api.ModelDisplayState.Trigger) - a separate endpoint from
// SetModelDisplay's PUT so triggering a touch reaction never has to
// also resend the current char_code/flip/pepper, and see
// ModelDisplay.Set's own doc comment for why doing so wouldn't have
// been safe anyway.
func (d *Data) TriggerModelDisplay(w http.ResponseWriter, r *http.Request) {
	d.ModelDisplay.IncrementTrigger()
	w.WriteHeader(http.StatusOK)
}

// SetModelDisplay overwrites the shared display state wholesale (PUT
// semantics, same as SetCardCharacterModel) - a controller always sends
// its full current {char_code, flip, pepper}, not a partial patch. An
// empty CharCode is accepted as a real, deliberate "nothing selected"
// state (e.g. clearing the display before a show starts), not an error -
// only a non-empty CharCode gets checked against character_models, so a
// typo'd code fails loudly here rather than the display silently
// getting stuck trying to load something that was never imported.
func (d *Data) SetModelDisplay(w http.ResponseWriter, r *http.Request) {
	var body api.ModelDisplayState
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

	d.ModelDisplay.Set(body)
	w.WriteHeader(http.StatusOK)
}
