package httpmw_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbgen"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

func TestExtractWorkspaceProxy(t *testing.T) {
	t.Parallel()

	successHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Only called if the API key passes through the handler.
		httpapi.Write(context.Background(), rw, http.StatusOK, codersdk.Response{
			Message: "It worked!",
		})
	})

	t.Run("NoHeader", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, "test:wow-hello")

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidID", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, "test:wow")

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecretLength", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", uuid.NewString(), "wow"))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()
		)

		secret, err := cryptorand.HexString(64)
		require.NoError(t, err)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", uuid.NewString(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, _ = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)

		// Use a different secret so they don't match!
		secret, err := cryptorand.HexString(64)
		require.NoError(t, err)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID.String(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(successHandler).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	})

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		var (
			db = dbfake.New()
			r  = httptest.NewRequest("GET", "/", nil)
			rw = httptest.NewRecorder()

			proxy, secret = dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		)
		r.Header.Set(httpmw.WorkspaceProxyAuthTokenHeader, fmt.Sprintf("%s:%s", proxy.ID.String(), secret))

		httpmw.ExtractWorkspaceProxy(httpmw.ExtractWorkspaceProxyConfig{
			DB: db,
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Checks that it exists on the context!
			_ = httpmw.WorkspaceProxy(r)
			successHandler.ServeHTTP(rw, r)
		})).ServeHTTP(rw, r)
		res := rw.Result()
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)
	})
}
