package helper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

var (
	allocOpts = append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU, chromedp.NoSandbox)
	browserOpts     []chromedp.ContextOption
	allocateOnce    sync.Once
	startServerOnce sync.Once
	allocCtx        context.Context
	browserCtx      context.Context
	testServer      *httptest.Server
	testdataDir     string
	testdataURL     string
)

func init() {
	wd, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("could not get working directory: %v", err))
	}
	testdataDir = filepath.Join(wd, "testdata")
	testdataURL = "file://" + path.Join(wd, "testdata")
}

func TestMain(m *testing.M) {
	var cancel context.CancelFunc
	allocCtx, cancel = chromedp.NewExecAllocator(context.Background(), allocOpts...)

	if debug := os.Getenv("DEBUG"); debug != "" && debug != "false" {
		browserOpts = append(browserOpts, chromedp.WithDebugf(log.Printf))
	}

	code := m.Run()
	cancel()

	if testServer != nil {
		testServer.Close()
	}

	os.Exit(code)
}

func testAllocate(tb testing.TB) (context.Context, context.CancelFunc) {
	allocateOnce.Do(func() {
		ctx, _ := chromedp.NewContext(allocCtx, browserOpts...)
		if err := chromedp.Run(ctx); err != nil {
			tb.Fatal(err)
		}
		chromedp.ListenBrowser(ctx, func(ev interface{}) {
			switch ev := ev.(type) {
			case *cdpruntime.EventExceptionThrown:
				tb.Errorf("%+v\n", ev.ExceptionDetails)
			}
		})
		browserCtx = ctx
	})

	if browserCtx == nil {
		tb.FailNow()
	}

	// Create new tab of existing browser.
	ctx, _ := chromedp.NewContext(browserCtx)
	cancel := func() {
		if err := chromedp.Cancel(ctx); err != nil {
			tb.Error(err)
		}
	}
	return ctx, cancel
}

func testStartServer(tb testing.TB) string {
	startServerOnce.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/image.png", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			http.ServeFile(w, r, filepath.Join(testdataDir, "image.png"))
		})
		mux.Handle("/", http.FileServer(http.Dir(testdataDir)))
		testServer = httptest.NewServer(mux)
	})

	if testServer == nil {
		tb.FailNow()
	}

	return testServer.URL
}

func TestScreenshot(t *testing.T) {
	t.Parallel()
	ctx, cancel := testAllocate(t)
	defer cancel()

	dir, err := ioutil.TempDir("", "chromedp-helper-test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	sspath := filepath.Join(dir, "screenshot.png")
	log.Println("sspath:", sspath)

	tasks := chromedp.Tasks{
		chromedp.Navigate(testdataURL + "/screenshot.html"),
		Screenshot(sspath),
	}
	if err := chromedp.Run(ctx, tasks); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(sspath)
	if err != nil {
		t.Fatalf("failed to open screenshot file: %v", err)
	}
	defer f.Close()

	config, format, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatalf("failed to decode image config: %v", err)
	}

	const wantFormat = "png"
	const wantWidth = 1200
	const wantHeight = 1234
	if format != wantFormat {
		t.Fatalf("expected format to be %q, got %q", wantFormat, format)
	}
	if config.Width != wantWidth || config.Height != wantHeight {
		t.Fatalf("expected dimensions to be %d*%d, got %d*%d",
			wantWidth, wantHeight, config.Width, config.Height)
	}
}

func TestNavigate(t *testing.T) {
	t.Parallel()
	ctx, cancel := testAllocate(t)
	defer cancel()
	endpoint := testStartServer(t)

	var got string
	tasks := chromedp.Tasks{
		network.Enable(),
		EnableLifeCycleEvents(),
		Navigate(endpoint+"/navigate.html", 5*time.Second),
		chromedp.Text("#text", &got),
	}
	if err := chromedp.Run(ctx, tasks); err != nil {
		t.Fatal(err)
	}
	const want = "DOMContentLoaded"
	if got != want {
		t.Fatalf("expected text to be %q, got %q", want, got)
	}
}

func TestIgnoreCacheReload(t *testing.T) {
	t.Parallel()
	ctx, cancel := testAllocate(t)
	defer cancel()
	endpoint := testStartServer(t)

	var got string
	tasks := chromedp.Tasks{
		network.Enable(),
		EnableLifeCycleEvents(),
		chromedp.Navigate(endpoint + "/navigate.html"),
		IgnoreCacheReload(5 * time.Second),
		chromedp.Text("#text", &got),
	}
	if err := chromedp.Run(ctx, tasks); err != nil {
		t.Fatal(err)
	}
	const want = "DOMContentLoaded"
	if got != want {
		t.Fatalf("expected text to be %q, got %q", want, got)
	}
}

func TestWaitResponse(t *testing.T) {
	t.Parallel()
	ctx, cancel := testAllocate(t)
	defer cancel()
	endpoint := testStartServer(t)

	var got string
	tasks := chromedp.Tasks{
		network.Enable(),
		EnableLifeCycleEvents(),
		chromedp.Navigate(endpoint + "/index.html"),
		WaitResponse(endpoint+"/navigate.html", 5*time.Second,
			chromedp.Click(`a[href="navigate.html"]`),
		),
		chromedp.Text("#text", &got),
	}
	if err := chromedp.Run(ctx, tasks); err != nil {
		t.Fatal(err)
	}
	const want = "DOMContentLoaded"
	if got != want {
		t.Fatalf("expected text to be %q, got %q", want, got)
	}
}

func TestWaitLoaded(t *testing.T) {
	t.Parallel()
	ctx, cancel := testAllocate(t)
	defer cancel()
	endpoint := testStartServer(t)

	var got string
	tasks := chromedp.Tasks{
		chromedp.Navigate(endpoint + "/index.html"),
		chromedp.Click(`a[href="navigate.html"]`),
		WaitLoaded(5 * time.Second),
		chromedp.Text("#text", &got),
	}
	if err := chromedp.Run(ctx, tasks); err != nil {
		t.Fatal(err)
	}
	const want = "loaded"
	if got != want {
		t.Fatalf("expected text to be %q, got %q", want, got)
	}
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
