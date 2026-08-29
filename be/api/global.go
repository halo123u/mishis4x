package api

type GlobalData struct {
	User User `json:"user"`
	// CollectionAccess mirrors handlers.Data.canAccessCollection for this
	// user - the frontend hides the Card Manager widget entirely when this
	// is false, rather than showing it and letting the user click through
	// to a 403 from GET /api/sets.
	CollectionAccess bool `json:"collection_access"`
}
