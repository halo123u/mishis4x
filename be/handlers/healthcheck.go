package handlers

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"
)

// Healthcheck reports whether this instance can actually serve traffic -
// specifically, whether its DB connection is alive. A pure liveness check
// (just returning 200) would let an orchestrator keep routing traffic to an
// instance whose DB connection died, since the process itself is still
// running.
func (d *Data) Healthcheck(w http.ResponseWriter, r *http.Request) {
	if d.P.DB == nil {
		log.Error().Msg("healthcheck: no db configured")
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()

	if err := d.P.DB.PingContext(ctx); err != nil {
		log.Error().Err(err).Msg("healthcheck: db unreachable")
		writeJSONError(w, http.StatusServiceUnavailable, "database unreachable")
		return
	}

	w.WriteHeader(http.StatusOK)
}
