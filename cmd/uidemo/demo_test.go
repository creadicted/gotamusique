package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/web"
)

// A smoke test of the demo: the real handlers on top of the fake bot behave like a
// bot, so the interface can be tried against it.
func TestDemoBehavesLikeABot(t *testing.T) {
	srv := httptest.NewServer(web.NewServer(newDemo(), web.NewAuth(web.AuthOptions{}), web.Options{
		AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181", AccessAddress: "http://127.0.0.1:8181", UploadEnabled: true,
	}).Handler())
	defer srv.Close()

	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Host = "127.0.0.1:8181"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]any
		json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}
	titles := func(s map[string]any) []string {
		var out []string
		for _, e := range s["queue"].([]any) {
			out = append(out, e.(map[string]any)["title"].(string))
		}
		return out
	}

	code, s := call("GET", "/api/v1/state", "")
	if code != 200 || len(titles(s)) != 4 || s["playing"] != true {
		t.Fatalf("initial state = %d %v", code, s)
	}

	code, s = call("POST", "/api/v1/playback/pause", "")
	if code != 200 || s["pause"] != "manual" {
		t.Errorf("pause = %d %v", code, s["pause"])
	}
	code, s = call("POST", "/api/v1/queue", `{"type":"radio","preset":"jazz","position":"next"}`)
	if code != 201 || len(titles(s)) != 5 {
		t.Fatalf("add = %d %v", code, titles(s))
	}
	id := int(s["added"].(map[string]any)["id"].(float64))
	if titles(s)[1] != "[Radio] jazz" {
		t.Errorf("a station added next should sit after the current entry: %v", titles(s))
	}
	code, s = call("POST", "/api/v1/queue/"+itoa(id)+"/move", `{"after":0}`)
	if code != 200 || titles(s)[0] != "[Radio] jazz" {
		t.Errorf("move to the front = %d %v", code, titles(s))
	}
	if code, _ = call("DELETE", "/api/v1/queue/"+itoa(id), ""); code != 200 {
		t.Errorf("remove = %d", code)
	}
	if code, s = call("DELETE", "/api/v1/queue/"+itoa(id), ""); code != 404 || s["code"] != "not_found" {
		t.Errorf("removing it again = %d %v, want the 404 clients rely on", code, s["code"])
	}
	if code, s = call("POST", "/api/v1/queue", `{"type":"url","url":"https://www.youtube.com/watch?v=x"}`); code != 422 {
		t.Errorf("a refused address = %d, want 422", code)
	}
	if code, s = call("GET", "/api/v1/library?q=miles", ""); code != 200 || s["total"] != float64(2) {
		t.Errorf("library search = %d %v", code, s["total"])
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
