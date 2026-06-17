package selfupdate

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeClient returns a canned response/error for Do.
type fakeClient struct {
	resp    *http.Response
	err     error
	calls   int
	lastURL string
}

func (f *fakeClient) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	f.lastURL = req.URL.String()
	return f.resp, f.err
}

func jsonResp(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

const releaseJSON = `{
  "tag_name": "v0.2.0",
  "assets": [
    {"name": "fleet_0.2.0_linux_amd64.tar.gz", "browser_download_url": "https://x/a.tar.gz"},
    {"name": "checksums.txt", "browser_download_url": "https://x/checksums.txt"}
  ]
}`

func TestCheckAvailable(t *testing.T) {
	fc := &fakeClient{resp: jsonResp(releaseJSON)}
	c := Checker{Repo: "braydend/fleet", Client: fc}
	res, err := c.Check("0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Available || res.Latest != "0.2.0" || res.Current != "0.1.0" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.Release.Assets) != 2 || res.Release.Assets[0].URL != "https://x/a.tar.gz" {
		t.Fatalf("assets not parsed: %+v", res.Release.Assets)
	}
	if !strings.Contains(fc.lastURL, "/repos/braydend/fleet/releases/latest") {
		t.Fatalf("wrong url: %s", fc.lastURL)
	}
}

func TestCheckUpToDate(t *testing.T) {
	fc := &fakeClient{resp: jsonResp(releaseJSON)}
	res, err := Checker{Repo: "braydend/fleet", Client: fc}.Check("0.2.0")
	if err != nil || res.Available {
		t.Fatalf("expected not available, got %+v err=%v", res, err)
	}
}

func TestCheckDevSkipsNetwork(t *testing.T) {
	fc := &fakeClient{err: io.ErrUnexpectedEOF} // would error if called
	res, err := Checker{Repo: "braydend/fleet", Client: fc}.Check("dev")
	if err != nil {
		t.Fatalf("dev check should not error, got %v", err)
	}
	if res.Available || fc.calls != 0 {
		t.Fatalf("dev check must not hit network: calls=%d res=%+v", fc.calls, res)
	}
}
