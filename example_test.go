package helper_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	helper "github.com/go-oss/chromedp-helper"
)

func Example_navigate() {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	ts := httptest.NewServer(exampleHandler())
	defer ts.Close()

	var next string
	var title string
	tasks := chromedp.Tasks{
		// It is necessary in the helper methods.
		network.Enable(),
		helper.EnableLifeCycleEvents(),

		helper.Navigate(ts.URL, time.Minute),
		chromedp.AttributeValue("//a[contains(., 'Link')]", "href", &next, nil),
		helper.Navigate(helper.URL(ts.URL, "%s", &next), time.Minute),
		chromedp.Title(&title),
	}
	err := chromedp.Run(ctx, tasks)
	if err != nil {
		panic(err)
	}
	fmt.Println(title)
	// Output: Example
}

func exampleHandler() http.Handler {
	index := strings.TrimSpace(`
<DOCTYPE html>
<html>
	<head>
		<title>Index</title>
	</head>
	<body>
		<a href="/next">Link</a>
	</body>
</html>`)
	next := strings.TrimSpace(`
<DOCTYPE html>
<html>
<head>
	<title>Example</title>
</head>
<body>
	Example
</body>
</html>`)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, index)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, next)
	})
	return mux
}
