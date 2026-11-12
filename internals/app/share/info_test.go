package share_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sade/internals/app/asset"
	"sade/internals/app/share"
	"sade/internals/testutil"
	"sade/internals/token"
)

// The recipient's page asks which state its link is in before it shows a
// player: works (with the file and sender), expired, or not a link at all.
func TestInfoReportsTheLinkState(t *testing.T) {
	signer, err := token.New(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	h := share.NewHandler(signer, fakeAssets{byID: map[string]*asset.Asset{"prev1": previewAsset()}}, localStore(t), testutil.Logger())
	h.UseDetails(func(id string) (share.Info, error) {
		return share.Info{FileName: "nunta.mp4", MediaType: "video", SizeBytes: 46, Sender: "ana@studio.ro"}, nil
	})
	srv := httptest.NewServer(http.HandlerFunc(h.Info))
	t.Cleanup(srv.Close)

	ask := func(tok string) (int, string) {
		resp, err := http.Get(srv.URL + "/?token=" + tok)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	good, _ := signer.Sign("preview", "prev1", time.Hour)
	code, body := ask(good)
	var info share.Info
	if code != http.StatusOK || json.Unmarshal([]byte(body), &info) != nil || info.Sender != "ana@studio.ro" || info.FileName != "nunta.mp4" {
		t.Fatalf("good link: %d %s", code, body)
	}

	expired, _ := signer.Sign("preview", "prev1", -time.Second)
	if code, body := ask(expired); code != http.StatusGone || !strings.Contains(body, "expired") {
		t.Errorf("expired link: %d %s", code, body)
	}

	download, _ := signer.Sign("download", "prev1", time.Hour) // wrong purpose
	for _, tok := range []string{"abc.def", download, ""} {
		if code, _ := ask(tok); code != http.StatusNotFound {
			t.Errorf("token %q: %d, want 404", tok, code)
		}
	}
}
