package api

// ModelDisplayState is both the GET and PUT body for
// /api/models/display - the shared "what should the physically-mounted
// display be showing right now" state for a Pepper's Ghost/acrylic rig,
// set by a controller (anyone browsing /models/{charCode}?broadcast=1)
// and polled by the display itself (a phone stuck inside the physical
// rig, with no practical way to interact with it directly - same
// reasoning as ?flip=/?pepper= on ModelViewer.tsx). CharCode empty
// means nothing has been set yet this server run - there's deliberately
// no persistence (see handlers.ModelDisplay's own doc comment), so a
// server restart clears it back to that state rather than resuming
// whatever was showing before.
type ModelDisplayState struct {
	CharCode string `json:"char_code"`
	Flip     string `json:"flip,omitempty"`
	Pepper   bool   `json:"pepper"`
	// Trigger is a plain incrementing counter, not a meaningful value on
	// its own - a controller's "Trigger touch" button bumps it (POST
	// /api/models/display/trigger, separate from this struct's own PUT
	// endpoint so triggering a touch never has to also resend
	// char_code/flip/pepper), and the display fires its own tap-to-
	// motion+audio reaction whenever it notices this changed since its
	// last poll. A counter rather than a boolean/timestamp: strictly
	// increasing means "did this change since I last looked" is a
	// single != comparison with no clock-skew or "was this already
	// true" ambiguity to worry about.
	Trigger int `json:"trigger"`
}

// ModelDisplayStatus is the GET /api/models/display/status response - a
// controller polls this (not ModelDisplayState's own GET endpoint) to
// drive its "Set as display" button's enabled state without itself
// counting as proof the display is alive (see
// handlers.ModelDisplay.Connected's own doc comment).
type ModelDisplayStatus struct {
	Connected bool `json:"connected"`
}
