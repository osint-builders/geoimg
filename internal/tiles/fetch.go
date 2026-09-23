// Package tiles talks to XYZ tile servers (Esri World Imagery by default):
// fetching with retries, classifying missing/placeholder tiles, and running
// bounded worker pools.
package tiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // tile decoders
	_ "image/png"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/gen2brain/webp" // WebP tile decoder

	"github.com/osint-builders/geoimg/internal/geo"
)

// DefaultURL is Esri's public World Imagery tile endpoint. Note the {z}/{y}/{x}
// (level/row/column) order used by ArcGIS REST tile services.
const DefaultURL = "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}"

// Attribution is the credit Esri requires wherever World Imagery is displayed.
const Attribution = "Esri, Maxar, Earthstar Geographics, and the GIS User Community"

// Status classifies a fetched tile.
type Status int

const (
	// OK means real imagery was returned.
	OK Status = iota
	// Missing means the server has no tile here (404/204/empty body).
	Missing
	// Placeholder means the server returned a "Map data not yet available" style filler tile.
	Placeholder
)

func (s Status) String() string {
	switch s {
	case OK:
		return "ok"
	case Missing:
		return "missing"
	case Placeholder:
		return "placeholder"
	}
	return "unknown"
}

// ErrFatal wraps HTTP responses that retrying cannot fix (bad token, forbidden, bad URL).
var ErrFatal = errors.New("tile server refused request")

// Fetcher downloads and decodes tiles.
type Fetcher struct {
	URL       string // template with {z}, {x}, {y}
	Token     string // optional ArcGIS access token / API key
	UserAgent string
	Retries   int
	Client    *http.Client

	Requests  atomic.Int64 // HTTP requests sent (including retries)
	Bytes     atomic.Int64 // response bytes received
	responses atomic.Int64 // HTTP responses of any status
}

// NewFetcher returns a Fetcher tuned for many small concurrent requests.
func NewFetcher(tmpl, token, userAgent string, workers int) *Fetcher {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = workers * 2
	tr.MaxIdleConnsPerHost = workers * 2
	tr.MaxConnsPerHost = workers
	tr.IdleConnTimeout = 90 * time.Second
	tr.ForceAttemptHTTP2 = true
	return &Fetcher{
		URL:       tmpl,
		Token:     token,
		UserAgent: userAgent,
		Retries:   4,
		Client:    &http.Client{Transport: tr, Timeout: 45 * time.Second},
	}
}

// TileURL expands the URL template for t.
func (f *Fetcher) TileURL(t geo.Tile) string {
	u := strings.NewReplacer(
		"{z}", strconv.Itoa(t.Z),
		"{x}", strconv.Itoa(t.X),
		"{y}", strconv.Itoa(t.Y),
	).Replace(f.URL)
	if f.Token != "" {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + "token=" + url.QueryEscape(f.Token)
	}
	return u
}

// Fetch downloads, decodes and classifies one tile. A nil error with Missing
// or Placeholder status means "no imagery here"; the image is still returned
// for placeholders so callers can use it as a last resort.
func (f *Fetcher) Fetch(ctx context.Context, t geo.Tile) (image.Image, Status, error) {
	data, status, err := f.get(ctx, f.TileURL(t))
	if err != nil || status != OK {
		return nil, status, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, Missing, fmt.Errorf("tile %s: decode: %w", t, err)
	}
	if IsPlaceholder(img) {
		return img, Placeholder, nil
	}
	return img, OK, nil
}

func (f *Fetcher) get(ctx context.Context, u string) ([]byte, Status, error) {
	var lastErr error
	for attempt := 0; attempt <= f.Retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, backoff(attempt, lastErr)); err != nil {
				return nil, Missing, err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return nil, Missing, fmt.Errorf("%w: %v", ErrFatal, err)
		}
		req.Header.Set("User-Agent", f.UserAgent)
		req.Header.Set("Accept", "image/webp,image/jpeg,image/png,image/*;q=0.8")
		f.Requests.Add(1)
		resp, err := f.Client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, Missing, ctx.Err()
			}
			// A server that has never answered is unreachable (DNS, refused,
			// proxy block): fail fast instead of retrying every tile.
			if f.responses.Load() == 0 && attempt >= 1 {
				return nil, Missing, fmt.Errorf("%w: %v", ErrFatal, err)
			}
			lastErr = err
			continue
		}
		f.responses.Add(1)
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		f.Bytes.Add(int64(len(body)))
		switch {
		case resp.StatusCode == http.StatusOK:
			if err != nil {
				lastErr = err
				continue
			}
			if len(body) == 0 {
				return nil, Missing, nil
			}
			return body, OK, nil
		case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusNoContent:
			return nil, Missing, nil
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
			lastErr = &retryAfterError{status: resp.StatusCode, wait: parseRetryAfter(resp.Header.Get("Retry-After"))}
			continue
		default:
			return nil, Missing, fmt.Errorf("%w: HTTP %d for %s", ErrFatal, resp.StatusCode, redact(u))
		}
	}
	return nil, Missing, fmt.Errorf("giving up on %s after %d attempts: %w", redact(u), f.Retries+1, lastErr)
}

type retryAfterError struct {
	status int
	wait   time.Duration
}

func (e *retryAfterError) Error() string { return fmt.Sprintf("HTTP %d", e.status) }

func backoff(attempt int, lastErr error) time.Duration {
	var ra *retryAfterError
	if errors.As(lastErr, &ra) && ra.wait > 0 {
		return min(ra.wait, 30*time.Second)
	}
	var ne net.Error
	base := 200 * time.Millisecond
	if errors.As(lastErr, &ne) && ne.Timeout() {
		base = 500 * time.Millisecond
	}
	d := base << (attempt - 1)
	return min(d+time.Duration(rand.Int64N(int64(d)/2+1)), 10*time.Second)
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func redact(u string) string {
	if i := strings.Index(u, "token="); i >= 0 {
		return u[:i] + "token=REDACTED"
	}
	return u
}
