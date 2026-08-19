package grpc

import (
	"testing"

	"github.com/norlis/httpgate/trace"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestTraceContextFromMD_ParsesValidTraceparent(t *testing.T) {
	tc := trace.New()
	md := metadata.Pairs(trace.Header, tc.Traceparent())
	got := traceContextFromMD(md)
	require.Equal(t, tc.TraceID, got.TraceID)
	require.NotEqual(t, tc.SpanID, got.SpanID, "local span must be fresh")
}

func TestTraceContextFromMD_GeneratesWhenMissingOrInvalid(t *testing.T) {
	for _, md := range []metadata.MD{{}, metadata.Pairs(trace.Header, "garbage")} {
		got := traceContextFromMD(md)
		require.Len(t, got.TraceID, 32)
		require.Len(t, got.SpanID, 16)
	}
}
