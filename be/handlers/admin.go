package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"example.com/mishis4x/api"
	"example.com/mishis4x/persist"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// ListPendingInvites is the web equivalent of `be invite-list` - every
// invite still awaiting an approve/deny decision, oldest first. Gated by
// adminOnlyMiddleware (see AdminUserID's doc comment), not exposed to
// any other authenticated user.
func (d *Data) ListPendingInvites(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	requests, err := d.P.ListRequestedInvites(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error listing invite requests")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	// Deliberately built as a separate api.AdminInviteRequest per row
	// rather than JSON-tagging persist.InviteRequest directly - see
	// api.AdminInviteRequest's doc comment for why the code must never
	// reach the browser.
	resp := make([]api.AdminInviteRequest, len(requests))
	for i, req := range requests {
		resp[i] = api.AdminInviteRequest{
			ID:           req.ID,
			EmailAddress: req.EmailAddress,
			CreatedAt:    req.CreatedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// inviteIDFromPath parses the {id} path var shared by approve/deny -
// both routes fail identically (400) on a non-numeric id, which
// shouldn't be reachable through the admin page's own UI (ids come from
// ListPendingInvites' own response) but is still real input from an
// untrusted client.
func inviteIDFromPath(r *http.Request) (int, error) {
	return strconv.Atoi(mux.Vars(r)["id"])
}

// ApproveInviteRequest is the web equivalent of `be invite-approve` -
// same underlying persist.ApproveInvite + email.Service.SendInviteEmail
// as the CLI command, just triggered by an admin's click instead of a
// terminal command. Requires EmailService and AppBaseURL to both be
// configured (see their doc comments on Data) - unlike the CLI, this
// can't fail before touching the DB and let the admin just fix the env
// and retry, since the request is already sitting there waiting; the
// invite still ends up 'approved' either way, with the response telling
// the admin whether the email actually went out.
func (d *Data) ApproveInviteRequest(w http.ResponseWriter, r *http.Request) {
	id, err := inviteIDFromPath(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid invite id.")
		return
	}

	if d.AppBaseURL == "" || d.EmailService == nil {
		log.Error().Msg("admin invite-approve: APP_BASE_URL/RESEND_API_KEY not configured")
		writeJSONError(w, http.StatusServiceUnavailable, "Email isn't configured on this server yet - approve via the invite-approve CLI command instead.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	req, err := d.P.ApproveInvite(ctx, id)
	if err != nil {
		if errors.Is(err, persist.ErrInviteNotPending) {
			writeJSONError(w, http.StatusConflict, "This request was already decided.")
			return
		}
		log.Error().Err(err).Int("id", id).Msg("error approving invite")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	signupURL := fmt.Sprintf("%s/sign-up?invite=%s", d.AppBaseURL, req.Code)
	if err := d.EmailService.SendInviteEmail(ctx, req.EmailAddress, signupURL); err != nil {
		// Already approved in the DB at this point - not rolled back,
		// same tradeoff as the CLI command. The admin can still share
		// the link manually - see be/cmd/invite.go's own doc comment.
		log.Error().Err(err).Str("email", req.EmailAddress).Msg("approved, but sending the invite email failed")
		writeJSONError(w, http.StatusInternalServerError, "Approved, but the email failed to send. Check the server logs for the link to share manually.")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DenyInviteRequest is the web equivalent of `be invite-deny`.
func (d *Data) DenyInviteRequest(w http.ResponseWriter, r *http.Request) {
	id, err := inviteIDFromPath(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid invite id.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	if _, err := d.P.DenyInvite(ctx, id); err != nil {
		if errors.Is(err, persist.ErrInviteNotPending) {
			writeJSONError(w, http.StatusConflict, "This request was already decided.")
			return
		}
		log.Error().Err(err).Int("id", id).Msg("error denying invite")
		writeJSONError(w, http.StatusInternalServerError, "Something went wrong.")
		return
	}

	w.WriteHeader(http.StatusOK)
}
