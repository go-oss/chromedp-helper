package helper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var (
	// ErrCanceledByUser is an error because of canceled by user.
	ErrCanceledByUser = errors.New("canceled by user")
)

// Screenshot is an action that takes a screenshot of the entire browser viewport and save as image file.
//
// Note: this will override the viewport emulation settings.
//
// This function is based on https://github.com/chromedp/examples
//
// filename can be specified by string, string pointer or fmt.Stringer.
func Screenshot(filename interface{}) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		// get layout metrics
		_, _, contentSize, err := page.GetLayoutMetrics().Do(ctx)
		if err != nil {
			return err
		}

		width, height := int64(math.Ceil(contentSize.Width)), int64(math.Ceil(contentSize.Height))

		// force viewport emulation
		err = emulation.SetDeviceMetricsOverride(width, height, 1, false).
			WithScreenOrientation(&emulation.ScreenOrientation{
				Type:  emulation.OrientationTypePortraitPrimary,
				Angle: 0,
			}).
			Do(ctx)
		if err != nil {
			return err
		}

		// capture screenshot
		res, err := page.CaptureScreenshot().
			WithQuality(100).
			WithClip(&page.Viewport{
				X:      contentSize.X,
				Y:      contentSize.Y,
				Width:  contentSize.Width,
				Height: contentSize.Height,
				Scale:  1,
			}).Do(ctx)
		if err != nil {
			return err
		}

		// save screenshot
		f, err := os.Create(toString(filename))
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.Write(res); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}

		return nil
	})
}

// Navigate is an action that navigates the current frame.
//
// urlstr can be specified by string, string pointer or fmt.Stringer.
func Navigate(urlstr interface{}, timeout time.Duration) chromedp.NavigateAction {
	return WaitResponse(urlstr, timeout,
		chromedp.ActionFunc(func(ctx context.Context) error {
			u := toString(urlstr)
			_, _, _, err := page.Navigate(u).Do(ctx)
			return err
		}),
	)
}

// IgnoreCacheReload is an action that reloads the current page without cache.
func IgnoreCacheReload(timeout time.Duration) chromedp.NavigateAction {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, entries, err := page.GetNavigationHistory().Do(ctx)
		if err != nil {
			return err
		}
		currentURL := entries[len(entries)-1].URL
		log.Printf("IgnoreCacheReload: current=%s\n", currentURL)
		return WaitResponse(currentURL, timeout,
			chromedp.ActionFunc(func(ctx context.Context) error {
				if err := page.Reload().WithIgnoreCache(true).Do(ctx); err != nil {
					return err
				}
				return nil
			}),
		).Do(ctx)
	})
}

// EnableLifeCycleEvents enables life cycle events.
func EnableLifeCycleEvents() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		err := page.Enable().Do(ctx)
		if err != nil {
			return err
		}
		err = page.SetLifecycleEventsEnabled(true).Do(ctx)
		if err != nil {
			return err
		}
		return nil
	})
}

// WaitResponse is an action that waits until response received or timeout exceeded.
//
// urlstr can be specified by string, string pointer or fmt.Stringer.
func WaitResponse(urlstr interface{}, timeout time.Duration, acts ...chromedp.Action) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		u := toString(urlstr)
		log.Printf("WaitResponse: wait for url=%s\n", u)
		ch := make(chan error, 1)
		reloadCh := make(chan struct{}, 1)
		lctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var requestID network.RequestID
		var loaderID cdp.LoaderID
		var frameID cdp.FrameID
		chromedp.ListenTarget(lctx, func(ev interface{}) {
			switch e := ev.(type) {
			// Wait request event
			case *network.EventRequestWillBeSent:
				req := e.Request
				if strings.HasPrefix(req.URL, u) {
					requestID = e.RequestID
					log.Printf("WaitResponse: request id=%s method=%s url=%s", e.RequestID, req.Method, req.URL)
				}

			// Handle network error
			case *network.EventLoadingFailed:
				if requestID == e.RequestID {
					log.Printf("WaitResponse: error=%s url=%s\n", e.ErrorText, u)
					select {
					case reloadCh <- struct{}{}:
					default:
					}
				}

			// Wait response
			case *network.EventResponseReceived:
				res := e.Response
				if strings.HasPrefix(res.URL, u) {
					log.Printf("WaitResponse: response status=%d url=%s\n", res.Status, res.URL)
					if res.Status >= 200 && res.Status < 400 {
						loaderID, frameID = e.LoaderID, e.FrameID
						return
					}
					switch res.Status {
					case http.StatusBadRequest:
						ch <- fmt.Errorf("status=BadRequest url=%s", u)
						return
					case http.StatusGone:
						ch <- fmt.Errorf("status=Gone url=%s", u)
						return
					}
					reloadCh <- struct{}{}
				}

			// Wait Loaded event
			case *page.EventLoadEventFired:
				select {
				case <-reloadCh:
					ch <- nil
				default:
					log.Println("WaitResponse: event=Load")
					cancel()
					close(ch)
				}

			// Wait DOMContentLoaded event
			case *page.EventLifecycleEvent:
				select {
				case <-reloadCh:
					ch <- nil
				default:
					switch e.Name {
					case "DOMContentLoaded":
						if e.LoaderID != loaderID || e.FrameID != frameID {
							return
						}
						log.Printf("WaitResponse: event=%s\n", e.Name)
						cancel()
						close(ch)
					}
				}
			}
		})
		log.Printf("WaitResponse: do action(s)=%d\n", len(acts))
		for _, a := range acts {
			if err := a.Do(ctx); err != nil {
				return err
			}
		}
		log.Printf("WaitResponse: timeout=%s\n", timeout)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case err, open := <-ch:
				if err != nil {
					return err
				}
				if open {
					log.Println("WaitResponse: reload")
					<-ticker.C
					if err := page.Reload().Do(ctx); err != nil {
						return err
					}
					continue
				}
				log.Println("WaitResponse: loaded")
				return nil
			case <-timer.C:
				log.Printf("WaitResponse: timeout exceeded url=%s\n", u)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})
}

// WaitLoaded is an action that waits until load event fired or timeout exceeded.
func WaitLoaded(timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		ch := make(chan struct{})
		lctx, cancel := context.WithCancel(ctx)
		defer cancel()
		chromedp.ListenTarget(lctx, func(ev interface{}) {
			if _, ok := ev.(*page.EventLoadEventFired); ok {
				close(ch)
			}
		})
		log.Printf("WaitLoaded: timeout=%s\n", timeout)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-ch:
			return nil
		case <-timer.C:
			log.Println("WaitLoaded: timeout exceeded")
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// WaitInput is an action that waits until input.
func WaitInput(r io.Reader, message string, expected ...string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		fmt.Print(message)
		ch := make(chan string)
		go func() {
			s := bufio.NewScanner(r)
			s.Scan()
			ch <- strings.TrimSpace(s.Text())
		}()
		select {
		case input := <-ch:
			if len(expected) == 0 {
				log.Println("WaitTerminalInput: Confirmed")
				return nil
			}
			for _, exp := range expected {
				if input == exp {
					log.Println("WaitTerminalInput: Confirmed")
					return nil
				}
			}
			log.Println("WaitTerminalInput: Canceled")
			return ErrCanceledByUser
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// WaitForTime is an action that waits until for time.
func WaitForTime(t time.Time) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		log.Printf("WaitForTime: %s\n", t)
		timer := time.NewTimer(time.Until(t))
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// SaveCookies is an action that saves cookies as json lines file.
//
// filename can be specified by string, string pointer or fmt.Stringer.
func SaveCookies(filename interface{}, maps ...func(*network.Cookie)) chromedp.Action {
	mapFunc := func(c *network.Cookie) {
		for _, m := range maps {
			m(c)
		}
	}
	return chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetAllCookies().Do(ctx)
		if err != nil {
			return err
		}

		log.Printf("SaveCookies: cookie(s)=%d\n", len(cookies))
		f, err := os.OpenFile(toString(filename), os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := f.Truncate(0); err != nil {
			return err
		}
		e := json.NewEncoder(f)
		for _, c := range cookies {
			mapFunc(c)
			e.Encode(c)
		}

		return f.Sync()
	})
}

// RestoreCookies is an action that restores cookies from json lines file.
//
// filename can be specified by string, string pointer or fmt.Stringer.
func RestoreCookies(filename interface{}, filters ...func(*network.Cookie) bool) chromedp.Action {
	filter := func(c network.Cookie) bool {
		for _, f := range filters {
			if !f(&c) {
				return false
			}
		}
		return true
	}
	return chromedp.ActionFunc(func(ctx context.Context) error {
		f, err := os.Open(toString(filename))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		defer f.Close()
		d := json.NewDecoder(f)
		cookies := make([]*network.Cookie, 0)
		for {
			var c network.Cookie
			if err := d.Decode(&c); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}
			if filter(c) {
				cookies = append(cookies, &c)
			}
		}
		log.Printf("RestoreCookies: cookie(s)=%d\n", len(cookies))

		// add cookies to browser
		for _, c := range cookies {
			expires := cdp.TimeSinceEpoch(time.Time{}.Add(time.Duration(c.Expires) * time.Second))
			success, err := network.SetCookie(c.Name, c.Value).
				WithDomain(c.Domain).
				WithPath(c.Path).
				WithSecure(c.Secure).
				WithHTTPOnly(c.HTTPOnly).
				WithSameSite(c.SameSite).
				WithExpires(&expires).
				WithPriority(c.Priority).
				Do(ctx)
			if err != nil {
				return err
			}
			if !success {
				return fmt.Errorf("could not set cookie %s to %s", c.Name, c.Value)
			}
		}
		return nil
	})
}
