package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"example.com/mishis4x/api"
	"github.com/stretchr/testify/require"
)

// TestGetGlobalData_ReflectsEbayListingsEnabled proves the frontend's
// signal for hiding the "eBay" option end-to-end - not just that the
// field exists in the response, but that it actually flips based on
// Data.EbayListingsDisabled.
func TestGetGlobalData_ReflectsEbayListingsEnabled(t *testing.T) {
	db := testDB(t)

	t.Run("enabled by default", func(t *testing.T) {
		ts, client := newTestServer(t, db)
		createAndLoginTestUser(t, db, client, ts.URL)

		res, err := client.Get(ts.URL + "/api/data")
		require.NoError(t, err)
		defer func() { _ = res.Body.Close() }()
		require.Equal(t, http.StatusOK, res.StatusCode)

		var data api.GlobalData
		require.NoError(t, json.NewDecoder(res.Body).Decode(&data))
		require.True(t, data.EbayListingsEnabled)
	})

	t.Run("disabled via the kill switch", func(t *testing.T) {
		ts, client := newTestServerWithEbayDisabled(t, db, nil)
		createAndLoginTestUser(t, db, client, ts.URL)

		res, err := client.Get(ts.URL + "/api/data")
		require.NoError(t, err)
		defer func() { _ = res.Body.Close() }()
		require.Equal(t, http.StatusOK, res.StatusCode)

		var data api.GlobalData
		require.NoError(t, json.NewDecoder(res.Body).Decode(&data))
		require.False(t, data.EbayListingsEnabled)
	})
}
