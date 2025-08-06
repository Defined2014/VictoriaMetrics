package rule

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmalert/datasource"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/auth"
	"github.com/stretchr/testify/require"
)

func TestRequestToCurl(t *testing.T) {
	f := func(req *http.Request, exp string) {
		t.Helper()
		got := requestToCurl(req)
		if got != exp {
			t.Fatalf("expected to have %q; got %q instead", exp, got)
		}
	}

	newReq := func(url string, queryParams ...string) *http.Request {
		t.Helper()
		r, err := http.NewRequest(http.MethodPost, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		params := r.URL.Query()
		for i := 0; i < len(queryParams); i += 2 {
			params.Add(queryParams[i], queryParams[i+1])
		}
		r.URL.RawQuery = params.Encode()

		return r
	}

	req := newReq("foo.com")
	f(req, "curl -X POST 'http://foo.com'")

	req = newReq("foo.com")
	req.Header.Set("foo", "bar")
	req.Header.Set("baz", "qux")
	f(req, "curl -X POST -H 'Baz: qux' -H 'Foo: bar' 'http://foo.com'")

	req = newReq("foo.com", "query", "up", "step", "10")
	f(req, "curl -X POST 'http://foo.com?query=up&step=10'")

	req = newReq("http://foo.com", "query", "up", "step", "10")
	f(req, "curl -X POST 'http://foo.com?query=up&step=10'")

	req = newReq("https://foo.com", "query", "up", "step", "10")
	f(req, "curl -k -X POST 'https://foo.com?query=up&step=10'")

	req = newReq("https://user:pass@foo.com", "query", "up", "step", "10")
	f(req, "curl -k -X POST 'https://user:xxxxx@foo.com?query=up&step=10'")

	req = newReq("https://user:pass@foo.com")
	req.Header.Set("Authorization", "Bearer 123456")
	f(req, "curl -k -X POST -H 'Authorization: <secret>' 'https://user:xxxxx@foo.com'")

	req = newReq("https://user:pass@foo.com")
	req.Header.Set("Authorization", "Basic 123456")
	f(req, "curl -k -X POST -H 'Authorization: <secret>' 'https://user:xxxxx@foo.com'")

	req = newReq("https://foo.com")
	req.Header.Set("My-Password", "mypassword")
	f(req, "curl -k -X POST -H 'My-Password: <secret>' 'https://foo.com'")

	req = newReq("https://foo.com")
	req.Header.Set("key-for", "my-new-key")
	f(req, "curl -k -X POST -H 'Key-For: <secret>' 'https://foo.com'")

	req = newReq("https://foo.com")
	req.Header.Set("My-Secret-Org", "secret-organization")
	f(req, "curl -k -X POST -H 'My-Secret-Org: <secret>' 'https://foo.com'")

	req = newReq("https://foo.com")
	req.Header.Set("Token", "secret-token")
	f(req, "curl -k -X POST -H 'Token: <secret>' 'https://foo.com'")
}

func TestAddTenantLabelToTSS(t *testing.T) {
	fq := &datasource.FakeQuerier{}
	fq.Add(metricWithValueAndLabels(t, 1, "__name__", "foo", "job", "bar"))

	auth, err := auth.NewToken("1:2")
	require.NoError(t, err)

	// with auth
	r := &RecordingRule{
		Name:           "test",
		q:              fq,
		state:          &ruleState{entries: make([]StateEntry, 10)},
		GroupAuthToken: auth,
	}
	tss, err := r.exec(context.Background(), time.Now(), 0)
	require.NoError(t, err)
	addTenantLabelToTSS(r.authToken(), tss)
	require.Len(t, tss[0].Labels, 4)
	require.Equal(t, tss[0].Labels[2].Name, "vm_account_id")
	require.Equal(t, tss[0].Labels[2].Value, "1")
	require.Equal(t, tss[0].Labels[3].Name, "vm_project_id")
	require.Equal(t, tss[0].Labels[3].Value, "2")

	// without auth
	r.GroupAuthToken = nil
	tss, err = r.exec(context.Background(), time.Now(), 0)
	require.NoError(t, err)
	addTenantLabelToTSS(r.authToken(), tss)
	require.Len(t, tss[0].Labels, 2)
}
