package handlers

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/rs/zerolog/log"
)

// ModelDisplay holds the shared "what should the physically-mounted
// display be showing right now" state for the model viewer's remote-
// control feature - a controller (anyone browsing
// /models/{charCode}?broadcast=1) writes to it, and a display (a phone
// stuck inside a physical Pepper's Ghost/acrylic rig, with no practical
// way to interact with it directly - see ModelViewer.tsx's own
// ?flip=/?pepper= for the same reasoning) polls it. Deliberately
// unpersisted, in-memory only, same single-instance-is-fine precedent
// as matchmaking.Lobby: this is ephemeral "what's on screen right now"
// state, not data worth surviving a restart, and this app already only
// ever runs as one instance.
//
// The mutex guards the whole state struct, not each field separately -
// a display mid-poll should never see e.g. a new CharCode paired with a
// stale Pepper value left over from before a concurrent write finished.
type ModelDisplay struct {
	mu    sync.Mutex
	state api.ModelDisplayState
}

func (d *ModelDisplay) Get() api.ModelDisplayState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

func (d *ModelDisplay) Set(state api.ModelDisplayState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state = state
}

// GetModelDisplay returns the current shared display state - the
// zero-value ModelDisplayState (empty CharCode) is the ordinary state
// for a server that's never had SetModelDisplay called yet this run,
// not an error.
func (d *Data) GetModelDisplay(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, d.ModelDisplay.Get())
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

		if _, err := d.P.GetCharacterModel(ctx, body.CharCode); err != nil {
			if errors.Is(err, persist.ErrCharacterModelNotFound) {
				writeJSONError(w, http.StatusNotFound, "Model not found.")
				return
			}
			log.Error().Err(err).Str("charCode", body.CharCode).Msg("error checking character model for display")
			writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
			return
		}
	}

	d.ModelDisplay.Set(body)
	w.WriteHeader(http.StatusOK)
}
