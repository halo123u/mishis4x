package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// canAccessCollection/ownerOnlyMiddleware aren't wired into any route right
// now (see CollectionOwnerUserID's doc comment) - kept in place for the
// planned market-rate feature to reuse rather than deleted. These are pure
// unit tests exercising them directly instead of through a real router/DB,
// since nothing currently routes through them to exercise otherwise.

func TestCanAccessCollection(t *testing.T) {
	cases := []struct {
		name        string
		ownerUserID int
		allowAll    bool
		requestID   int
		want        bool
	}{
		{"owner unset fails closed, even for a real user", 0, false, 42, false},
		{"owner unset fails closed for id 0 too", 0, false, 0, false},
		{"matching owner passes", 7, false, 7, true},
		{"non-matching user fails", 7, false, 8, false},
		{"allow-all overrides everything, no owner configured", 0, true, 12345, true},
		{"allow-all overrides even a mismatched owner", 7, true, 8, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Data{CollectionOwnerUserID: c.ownerUserID, CollectionAllowAllUsers: c.allowAll}
			require.Equal(t, c.want, d.canAccessCollection(c.requestID))
		})
	}
}

func TestOwnerOnlyMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("no user in context is forbidden, not unauthorized", func(t *testing.T) {
		d := Data{CollectionOwnerUserID: 7}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		d.ownerOnlyMiddleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("matching owner passes through to next", func(t *testing.T) {
		d := Data{CollectionOwnerUserID: 7}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDContextKey{}, 7))
		rec := httptest.NewRecorder()
		d.ownerOnlyMiddleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("non-matching user is forbidden", func(t *testing.T) {
		d := Data{CollectionOwnerUserID: 7}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDContextKey{}, 8))
		rec := httptest.NewRecorder()
		d.ownerOnlyMiddleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("allow-all lets any user through", func(t *testing.T) {
		d := Data{CollectionAllowAllUsers: true}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDContextKey{}, 999))
		rec := httptest.NewRecorder()
		d.ownerOnlyMiddleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	})
}
