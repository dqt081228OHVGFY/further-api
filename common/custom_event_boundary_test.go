package common

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type rejectedEventWriter struct{ failure error }

func (writer rejectedEventWriter) Write(_ []byte) (int, error) { return 0, writer.failure }

func TestCustomEventHandlesNonStringDataAndPropagatesDisconnect(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, encode(&output, CustomEvent{Data: 42}))
	require.Equal(t, "42", output.String())
	output.Reset()
	require.NoError(t, encode(&output, CustomEvent{Data: "data: hello"}))
	require.Equal(t, "data: hello\n\n", output.String())
	failure := errors.New("local writer disconnected")
	require.ErrorIs(t, encode(rejectedEventWriter{failure}, CustomEvent{Data: "data: hello"}), failure)
}
