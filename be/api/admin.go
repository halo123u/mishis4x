package api

import "time"

// AdminInviteRequest is deliberately its own type rather than a direct
// JSON-tagged persist.InviteRequest - it must never include the
// invite's code. The admin doesn't need to see it (approving sends it
// by email directly), and there's no reason for a sensitive bearer
// credential to ever reach the browser's network tab/DOM at all.
type AdminInviteRequest struct {
	ID           int       `json:"id"`
	EmailAddress string    `json:"email_address"`
	CreatedAt    time.Time `json:"created_at"`
}
