package context_tests

import (
	"context"
	"testing"
	"time"
)

func Test_ReturnOnContextCancel(t *testing.T) {
	testCases := []struct {
		desc  string
		given func(context.Context)
		want  bool
	}{
		{
			desc: "it should fail on non-returning function",
			given: func(ctx context.Context) {
				// be quite, gopls
				_ = ctx
				lock := make(chan struct{})
				<-lock
			},
			want: false,
		},
		{
			desc: "it should pass on returning function",
			given: func(ctx context.Context) {
				select {
				case <-ctx.Done():
				}
			},
			want: true,
		},
	}
	testCaseTimeout := 10 * time.Millisecond
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			got := testPass(tC.given, testCaseTimeout)
			if got != tC.want {
				t.Fatalf("wanted: %v, got: %v", tC.want, got)
			}
		})
	}
}
