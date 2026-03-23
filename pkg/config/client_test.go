package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- StaticSource ---

func TestStaticSource_Load(t *testing.T) {
	src := NewStaticSource(map[string]string{
		"otp_expiry":  "300",
		"max_retries": "5",
	})
	vals, err := src.Load(context.Background(), []string{"otp_expiry", "max_retries", "missing"})
	require.NoError(t, err)
	assert.Equal(t, "300", vals["otp_expiry"])
	assert.Equal(t, "5", vals["max_retries"])
	assert.Empty(t, vals["missing"])
}

// --- Watcher getters ---

func newTestWatcher(t *testing.T, values map[string]string) *Watcher {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	w, err := NewWatcher(
		NewStaticSource(values),
		nil,
		10*time.Minute,
		logger,
	)
	require.NoError(t, err)
	return w
}

func TestWatcher_Get(t *testing.T) {
	w := newTestWatcher(t, map[string]string{"foo": "bar"})
	w.Set("foo", "bar")
	assert.Equal(t, "bar", w.Get("foo"))
	assert.Equal(t, "", w.Get("missing"))
}

func TestWatcher_GetString(t *testing.T) {
	w := newTestWatcher(t, nil)
	w.Set("lang", "hi")
	assert.Equal(t, "hi", w.GetString("lang", "en"))
	assert.Equal(t, "en", w.GetString("missing", "en"))
}

func TestWatcher_GetInt(t *testing.T) {
	w := newTestWatcher(t, nil)
	w.Set("timeout", "30")
	assert.Equal(t, 30, w.GetInt("timeout", 0))
	assert.Equal(t, 99, w.GetInt("missing", 99))
	w.Set("bad", "not-a-number")
	assert.Equal(t, 10, w.GetInt("bad", 10))
}

func TestWatcher_GetBool(t *testing.T) {
	w := newTestWatcher(t, nil)
	for _, v := range []string{"true", "1", "yes", "TRUE", "YES"} {
		w.Set("flag", v)
		assert.True(t, w.GetBool("flag", false), "expected true for %q", v)
	}
	for _, v := range []string{"false", "0", "no", "off"} {
		w.Set("flag", v)
		assert.False(t, w.GetBool("flag", true), "expected false for %q", v)
	}
	assert.True(t, w.GetBool("missing", true))
	assert.False(t, w.GetBool("missing", false))
}

func TestWatcher_Set(t *testing.T) {
	w := newTestWatcher(t, nil)
	w.Set("key", "value1")
	assert.Equal(t, "value1", w.Get("key"))
	w.Set("key", "value2")
	assert.Equal(t, "value2", w.Get("key"))
}

// --- Watcher.Start with StaticSource ---

func TestWatcher_Start_LoadsOnStart(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	w, err := NewWatcher(
		NewStaticSource(map[string]string{"k": "v"}),
		[]string{"k"},
		1*time.Hour,
		logger,
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	// Give watcher time to do the initial load.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, "v", w.Get("k"))

	<-done
}

func TestWatcher_Start_RefreshesOnInterval(t *testing.T) {
	counter := 0
	src := &countingSource{values: map[string]string{"x": "1"}, counter: &counter}

	logger, _ := zap.NewDevelopment()
	w, err := NewWatcher(src, []string{"x"}, 50*time.Millisecond, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	<-done

	// Should have called Load at least 3 times (initial + 2+ ticks at 50ms in 250ms).
	assert.GreaterOrEqual(t, counter, 3)
}

type countingSource struct {
	values  map[string]string
	counter *int
}

func (c *countingSource) Load(_ context.Context, _ []string) (map[string]string, error) {
	*c.counter++
	return c.values, nil
}

// --- HTTPSource ---

func TestHTTPSource_Load(t *testing.T) {
	expected := map[string]string{"otp_expiry": "300", "max_attempts": "5"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		// Verify keys were sent as query params.
		keys := r.URL.Query()["key"]
		assert.ElementsMatch(t, []string{"otp_expiry", "max_attempts"}, keys)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	src := NewHTTPSource(srv.URL)
	got, err := src.Load(context.Background(), []string{"otp_expiry", "max_attempts"})
	require.NoError(t, err)
	assert.Equal(t, "300", got["otp_expiry"])
	assert.Equal(t, "5", got["max_attempts"])
}

func TestHTTPSource_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := NewHTTPSource(srv.URL)
	_, err := src.Load(context.Background(), []string{"key"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestHTTPSource_InvalidURL(t *testing.T) {
	src := NewHTTPSource("://bad-url")
	_, err := src.Load(context.Background(), []string{"key"})
	assert.Error(t, err)
}

func TestHTTPSource_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	src := NewHTTPSource(srv.URL)
	_, err := src.Load(context.Background(), []string{"key"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}
