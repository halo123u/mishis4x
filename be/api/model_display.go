package api

// ModelDisplayState is the GET /api/models/display response - the
// shared "what should the physically-mounted display be showing right
// now" state for a Pepper's Ghost/acrylic rig, set piecemeal by a
// controller (anyone browsing /models/{charCode}) via three separate,
// narrowly-scoped write endpoints rather than one PUT of this whole
// struct - see handlers.ModelDisplay's Set/SetFlip/IncrementTrigger for
// why each field has its own writer. CharCode empty means nothing has
// been set yet this server run - there's deliberately no persistence
// (see handlers.ModelDisplay's own doc comment), so a server restart
// clears it back to that state rather than resuming whatever was
// showing before.
type ModelDisplayState struct {
	CharCode string `json:"char_code"`
	Flip     string `json:"flip,omitempty"`
	// Trigger is a plain incrementing counter, not a meaningful value on
	// its own - a controller's "Trigger touch" button bumps it (POST
	// /api/models/display/trigger), and the display fires its own tap-
	// to-motion+audio reaction whenever it notices this changed since
	// its last poll. A counter rather than a boolean/timestamp: strictly
	// increasing means "did this change since I last looked" is a
	// single != comparison with no clock-skew or "was this already
	// true" ambiguity to worry about.
	Trigger int `json:"trigger"`
	// OffsetX/OffsetY/Zoom are a screen-space pan/zoom correction applied
	// on top of whatever the character/flip is already rendering -
	// independent of which character is showing (see
	// SetModelDisplayTransformInput's own doc comment for why this isn't
	// folded into char_code/flip at all). Zero-value (0, 0, 0) is the
	// ordinary "never touched" state for a server that's never had
	// SetModelDisplayTransform called yet this run, same as CharCode's
	// empty-string default - the frontend treats a zero/omitted Zoom as
	// 1 (no zoom), not literally 0 (which would render nothing).
	OffsetX int     `json:"offset_x,omitempty"`
	OffsetY int     `json:"offset_y,omitempty"`
	Zoom    float64 `json:"zoom,omitempty"`
}

// SetModelDisplayCharCodeInput is the PUT /api/models/display request
// body ("Set as display") - sets which character the display shows,
// and its starting orientation in the same action. Deliberately not
// ModelDisplayState itself: this endpoint only ever touches CharCode
// and Flip together (see handlers.ModelDisplay.Set), never Trigger, so
// there's no zero-value-Trigger field for a caller to accidentally
// resend and reset.
type SetModelDisplayCharCodeInput struct {
	CharCode string `json:"char_code"`
	Flip     string `json:"flip,omitempty"`
}

// SetModelDisplayFlipInput is the PUT /api/models/display/flip request
// body - adjusts just the display's current orientation, independent of
// which character is showing. Its own endpoint (not folded into
// SetModelDisplayCharCodeInput's PUT) specifically so live-tweaking
// orientation from a controller never has to know or resend whichever
// character actually happens to be live on the display right now - see
// handlers.ModelDisplay.SetFlip's own doc comment.
type SetModelDisplayFlipInput struct {
	Flip string `json:"flip,omitempty"`
}

// SetModelDisplayTransformInput is the PUT /api/models/display/transform
// request body - a controller's pan/zoom nudge buttons, correcting where
// the character sits in frame on the physical rig. Its own endpoint, own
// writer (handlers.ModelDisplay.SetTransform), and deliberately not part
// of SetModelDisplayCharCodeInput at all (unlike Flip, which is both an
// initial value on "Set as display" and its own live-tweak endpoint):
// this pan/zoom correction is calibrating the physical mounting itself,
// not something that means anything different per character, so it has
// no business resetting - or being resent - just because a different
// character got picked.
type SetModelDisplayTransformInput struct {
	OffsetX int     `json:"offset_x,omitempty"`
	OffsetY int     `json:"offset_y,omitempty"`
	Zoom    float64 `json:"zoom,omitempty"`
}

// ModelDisplayStatus is the GET /api/models/display/status response - a
// controller polls this (not ModelDisplayState's own GET endpoint) to
// drive its "Set as display" button's enabled state without itself
// counting as proof the display is alive (see
// handlers.ModelDisplay.Connected's own doc comment).
type ModelDisplayStatus struct {
	Connected bool `json:"connected"`
}
