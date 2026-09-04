package api

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Status   string `json:"status"`
	// IsAdmin is computed (userID == handlers.Data.AdminUserID), not a
	// stored column - see GetGlobalData. Gates whether the frontend shows
	// the admin invite-approval page's nav link at all; the routes
	// themselves are independently gated server-side too.
	IsAdmin bool `json:"is_admin"`
}

type UserLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
