package proxy

import (
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/coffee-realist/balancer/internal/logger"
	"github.com/stretchr/testify/assert"
)

type stubBalancer struct{ server string }

func (s *stubBalancer) Next() string    { return s.server }
func (s *stubBalancer) Increase(string) {}
func (s *stubBalancer) Decrease(string) {}
func (s *stubBalancer) Stop()           {}

func setup(t *testing.T) (handler http.Handler, payload string, backendURL string) {
	gofakeit.Seed(0)
	payload = gofakeit.Sentence(10)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/some/path", r.URL.Path)
		assert.Equal(t, "x=1&y=2", r.URL.RawQuery)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(backend.Close)

	log := logger.New()
	sb := &stubBalancer{server: backend.URL}
	handler = NewProxy(sb, log).Handler()
	return handler, payload, backend.URL
}
func TestProxyForwardsRequests(t *testing.T) {
	handler, expectedBody, backendURL := setup(t)

	t.Run("status and body are proxied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, backendURL+"/some/path?x=1&y=2", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			require.NoError(t, err)
		}(resp.Body)

		body, _ := io.ReadAll(resp.Body)

		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		assert.Equal(t, expectedBody, string(body))
	})

	t.Run("method is preserved", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, backendURL+"/some/path?x=1&y=2", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusAccepted, rec.Result().StatusCode)
	})

	t.Run("backend unreachable → 502", func(t *testing.T) {
		sb := &stubBalancer{server: "http://127.0.0.1:1"}
		handler := NewProxy(sb, logger.New()).Handler()

		req := httptest.NewRequest(http.MethodGet, "http://any/foo", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadGateway, rec.Result().StatusCode)
		body, _ := io.ReadAll(rec.Result().Body)
		assert.Contains(t, string(body), "bad gateway")
	})
}
func BenchmarkProxy(b *testing.B) {
	handler, _, backendURL := setup(&testing.T{})

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, backendURL+"/some/path?x=1&y=2", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
