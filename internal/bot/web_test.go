package bot

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/web"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func webBot(t *testing.T, mutate func(*config.WebConfig)) *Bot {
	t.Helper()
	cfg := defaultCfg()
	cfg.Web = config.WebConfig{
		Enabled: true, Addr: "127.0.0.1", Port: freePort(t),
		AuthMethod: config.AuthToken,
	}
	cfg.Web.AccessAddress = "http://127.0.0.1:" + strconv.Itoa(cfg.Web.Port)
	if mutate != nil {
		mutate(&cfg.Web)
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWebLogin_Disabled(t *testing.T) {
	cfg := defaultCfg()
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.WebLogin(web.Identity{Subject: "s", Name: "n"}); !errors.Is(err, web.ErrDisabled) {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
}

func TestWebLogin_NoLoginReturnsThePlainAddress(t *testing.T) {
	b := webBot(t, func(w *config.WebConfig) { w.AuthMethod = config.AuthNone })
	link, ttl, err := b.WebLogin(web.Identity{Subject: "s", Name: "n"})
	if err != nil || ttl != 0 || !strings.HasPrefix(link, "http://127.0.0.1:") || strings.Contains(link, "token") {
		t.Errorf("= %q, %v, %v", link, ttl, err)
	}
}

func TestStartWeb_DisabledDoesNothing(t *testing.T) {
	cfg := defaultCfg()
	cfg.Web.Port = freePort(t)
	b, _ := New(cfg)
	if err := b.StartWeb(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(cfg.Web.Port)); err != nil {
		t.Errorf("the port is taken although the web interface is disabled: %v", err)
	} else {
		ln.Close()
	}
}

func TestStartWeb_ReportsABusyPortSynchronously(t *testing.T) {
	b := webBot(t, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(b.cfg.Web.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := b.StartWeb(context.Background()); err == nil {
		t.Error("expected an error for a port that is already in use")
	}
}

// The whole path: !web issues a link, opening it logs in, and the API then
// serves the bot's real state.
func TestWeb_EndToEnd(t *testing.T) {
	b := webBot(t, nil)
	b.cfg.Bot.MaxVolume = 1
	enqueue3(b)
	_ = b.Play(1)
	b.SetVolume(40)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := b.StartWeb(ctx); err != nil {
		t.Fatal(err)
	}
	base := b.cfg.Web.AccessAddress

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	resp, err := c.Get(base + "/api/v1/state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("state before login = %d, want 401", resp.StatusCode)
	}

	link, ttl, err := b.WebLogin(web.Identity{Subject: "h", Name: "alice", Roles: []web.Role{web.RoleUser}})
	if err != nil || ttl != web.DefaultTokenTTL {
		t.Fatalf("WebLogin = %q, %v, %v", link, ttl, err)
	}
	resp, err = c.Get(link) // follows the redirect to /
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	if resp, _ := c.Get(link); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a login link must work once, second use = %d", resp.StatusCode)
	}

	resp, err = c.Get(base + "/api/v1/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s struct {
		Version string `json:"version"`
		Me      struct{ Name string }
		Queue   []struct {
			ID    uint64
			Title string
		}
		Index  int
		Volume float64
		Mode   string
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || s.Me.Name != "alice" || len(s.Queue) != 3 || s.Index != 1 ||
		s.Volume != 0.4 || s.Mode != "one-shot" || s.Queue[1].Title != "[Radio] b" {
		t.Errorf("state = %+v (status %d)", s, resp.StatusCode)
	}

	// Polling with the version just seen costs a 304.
	req, _ := http.NewRequest("GET", base+"/api/v1/state", nil)
	req.Header.Set("If-None-Match", resp.Header.Get("ETag"))
	r2, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusNotModified {
		t.Errorf("unchanged poll = %d, want 304", r2.StatusCode)
	}
	b.SetVolume(50)
	r3, _ := c.Do(req)
	r3.Body.Close()
	if r3.StatusCode != http.StatusOK {
		t.Errorf("poll after a change = %d, want 200", r3.StatusCode)
	}
}
