package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/codersdk"
)

type testOAuth2Provider struct {
	t        testing.TB
	authOpts []oauth2.AuthCodeOption
}

func (p *testOAuth2Provider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	assert.EqualValues(p.t, p.authOpts, opts)
	return "?state=" + url.QueryEscape(state)
}

func (*testOAuth2Provider) Exchange(_ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: "hello",
	}, nil
}

func (*testOAuth2Provider) TokenSource(_ context.Context, _ *oauth2.Token) oauth2.TokenSource {
	return nil
}

func newTestOAuth2Provider(t testing.TB, opts ...oauth2.AuthCodeOption) *testOAuth2Provider {
	return &testOAuth2Provider{
		t:        t,
		authOpts: opts,
	}
}

// nolint:bodyclose
func TestOAuth2(t *testing.T) {
	t.Parallel()
	t.Run("NotSetup", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/", nil)
		res := httptest.NewRecorder()
		httpmw.ExtractOAuth2(nil, nil, nil)(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})
	t.Run("RedirectWithoutCode", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?redirect="+url.QueryEscape("/dashboard"), nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, nil)(nil).ServeHTTP(res, req)
		location := res.Header().Get("Location")
		if !assert.NotEmpty(t, location) {
			return
		}
		require.Len(t, res.Result().Cookies(), 2)
		cookie := res.Result().Cookies()[1]
		require.Equal(t, "/dashboard", cookie.Value)
	})
	t.Run("NoState", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=something", nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, nil)(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})
	t.Run("NoStateCookie", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=something&state=test", nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, nil)(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusUnauthorized, res.Result().StatusCode)
	})
	t.Run("MismatchedState", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=something&state=test", nil)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.OAuth2StateCookie,
			Value: "mismatch",
		})
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, nil)(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusUnauthorized, res.Result().StatusCode)
	})
	t.Run("ExchangeCodeAndState", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=test&state=something", nil)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.OAuth2StateCookie,
			Value: "something",
		})
		req.AddCookie(&http.Cookie{
			Name:  "oauth_redirect",
			Value: "/dashboard",
		})
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, nil)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			state := httpmw.OAuth2(r)
			require.Equal(t, "/dashboard", state.Redirect)
		})).ServeHTTP(res, req)
	})
	t.Run("CustomAuthCodeOptions", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?redirect="+url.QueryEscape("/dashboard"), nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("foo", "bar"))
		authOpts := map[string]string{"foo": "bar"}
		httpmw.ExtractOAuth2(tp, nil, authOpts)(nil).ServeHTTP(res, req)
		location := res.Header().Get("Location")
		// Ideally we would also assert that the location contains the query params
		// we set in the auth URL but this would essentially be testing the oauth2 package.
		// testOAuth2Provider does this job for us.
		require.NotEmpty(t, location)
	})
}
