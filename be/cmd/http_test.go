package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePort(t *testing.T) {
	port, err := parsePort("")
	require.NoError(t, err)
	require.Equal(t, defaultPort, port)

	port, err = parsePort("9090")
	require.NoError(t, err)
	require.Equal(t, 9090, port)

	_, err = parsePort("not-a-number")
	require.Error(t, err)

	_, err = parsePort("0")
	require.Error(t, err)

	_, err = parsePort("-1")
	require.Error(t, err)

	_, err = parsePort("70000")
	require.Error(t, err)
}
