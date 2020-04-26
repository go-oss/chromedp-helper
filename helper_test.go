package helper

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestScreenshot(t *testing.T) {
	t.Skip("TODO")
}

func TestNavigate(t *testing.T) {
	t.Skip("TODO")
}

func TestIgnoreCacheReload(t *testing.T) {
	t.Skip("TODO")
}

func TestEnableLifeCycleEvents(t *testing.T) {
	t.Skip("TODO")
}

func TestWaitResponse(t *testing.T) {
	t.Skip("TODO")
}

func TestWaitLoaded(t *testing.T) {
	t.Skip("TODO")
}

func TestWaitInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		r        io.Reader
		message  string
		expected []string
		ctx      context.Context
		want     error
	}{
		{
			name: "no checking",
			r: func() io.Reader {
				return bytes.NewBufferString("\n")
			}(),
			expected: nil,
			ctx:      context.Background(),
			want:     nil,
		},
		{
			name: "yes",
			r: func() io.Reader {
				return bytes.NewBufferString("Y\n")
			}(),
			expected: []string{"Y", "y"},
			ctx:      context.Background(),
			want:     nil,
		},
		{
			name: "yes (small letter)",
			r: func() io.Reader {
				return bytes.NewBufferString("y\n")
			}(),
			expected: []string{"Y", "y"},
			ctx:      context.Background(),
			want:     nil,
		},
		{
			name: "no",
			r: func() io.Reader {
				return bytes.NewBufferString("n\n")
			}(),
			expected: []string{"Y", "y"},
			ctx:      context.Background(),
			want:     ErrCanceledByUser,
		},
		{
			name: "context canceled",
			r: func() io.Reader {
				return bytes.NewBufferString("n\n")
			}(),
			expected: []string{"Y", "y"},
			ctx:      context.Background(),
			want:     ErrCanceledByUser,
		},
		{
			name: "context canceled",
			r: func() io.Reader {
				return bytes.NewBufferString("")
			}(),
			expected: []string{"Y", "y"},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			want: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(tt.ctx, time.Second)
			defer cancel()
			got := WaitInput(tt.r, tt.message, tt.expected...).Do(ctx)
			if !errors.Is(got, tt.want) {
				t.Fatalf("%#v != %#v", got, tt.want)
			}
		})
	}
}

func TestWaitForTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		t    time.Time
		ctx  context.Context
		want error
	}{
		{
			name: "past time",
			t:    time.Now().Add(-time.Hour),
			ctx:  context.Background(),
			want: nil,
		},
		{
			name: "future time",
			t:    time.Now().Add(300 * time.Millisecond),
			ctx:  context.Background(),
			want: nil,
		},
		{
			name: "context canceled",
			t:    time.Now().Add(time.Hour),
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				return ctx
			}(),
			want: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(tt.ctx, time.Second)
			defer cancel()
			got := WaitForTime(tt.t).Do(ctx)
			if got != tt.want {
				t.Fatalf("%#v != %#v", got, tt.want)
			}
		})
	}
}
