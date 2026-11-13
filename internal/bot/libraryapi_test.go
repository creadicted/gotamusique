package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/database"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/media/file"
	"github.com/konradk/gotamusique/internal/web"
)

// browseBot has a small tree:
//
//	a.mp3
//	jazz/q.mp3
//	rock/x.mp3  (fav)
//	rock/y.mp3
//	rock/metal/z.mp3  (fav, loud)
func browseBot(t *testing.T) *libBot {
	t.Helper()
	lb := newLibBot(t, 0, nil)
	for _, p := range []string{"a.mp3", "jazz/q.mp3", "rock/x.mp3", "rock/y.mp3", "rock/metal/z.mp3"} {
		lb.addTrack(t, p, true)
	}
	tag := func(path string, tags ...string) {
		if err := lb.libraryRef().db.UpdateTags(file.IDFromPath(path), func([]string) []string { return tags }); err != nil {
			t.Fatal(err)
		}
	}
	tag("rock/x.mp3", "fav")
	tag("rock/metal/z.mp3", "fav", "loud")
	return lb
}

func paths(ts []library.Track) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.Path)
	}
	return out
}

func dirNames(ds []web.LibraryDir) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Path+":"+string(rune('0'+d.Tracks)))
	}
	return strings.Join(out, ",")
}

func TestLibrary_BrowseTheRoot(t *testing.T) {
	lb := browseBot(t)
	p, err := lb.Library(web.LibraryQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got := dirNames(p.Dirs); got != "jazz:1,rock:3" {
		t.Errorf("dirs = %s, want jazz:1,rock:3 (counts include sub-folders)", got)
	}
	if got := strings.Join(paths(p.Tracks), ","); got != "a.mp3" || p.Total != 1 {
		t.Errorf("tracks = %s total %d, want only the file in the root", got, p.Total)
	}
	if strings.Join(p.Tags, ",") != "fav,loud" {
		t.Errorf("tags = %v", p.Tags)
	}
}

func TestLibrary_BrowseAFolder(t *testing.T) {
	lb := browseBot(t)
	p, _ := lb.Library(web.LibraryQuery{Dir: "rock", Limit: 100})
	if got := dirNames(p.Dirs); got != "rock/metal:1" {
		t.Errorf("dirs = %s", got)
	}
	if got := strings.Join(paths(p.Tracks), ","); got != "rock/x.mp3,rock/y.mp3" || p.Total != 2 {
		t.Errorf("tracks = %s total %d", got, p.Total)
	}
	// "ro" is not the folder "rock": a prefix must end at a path separator.
	if p, _ := lb.Library(web.LibraryQuery{Dir: "ro", Limit: 100}); len(p.Tracks) != 0 || len(p.Dirs) != 0 {
		t.Errorf("a partial folder name matched: %+v", p)
	}
	if p, _ := lb.Library(web.LibraryQuery{Dir: "nope", Limit: 100}); len(p.Tracks) != 0 || len(p.Dirs) != 0 || p.Total != 0 {
		t.Errorf("an unknown folder = %+v, want empty", p)
	}
}

func TestLibrary_SearchLooksBelowTheFolder(t *testing.T) {
	lb := browseBot(t)
	p, _ := lb.Library(web.LibraryQuery{Search: "rock", Limit: 100})
	if got := strings.Join(paths(p.Tracks), ","); got != "rock/metal/z.mp3,rock/x.mp3,rock/y.mp3" || p.Total != 3 {
		t.Errorf("search = %s total %d", got, p.Total)
	}
	if len(p.Dirs) != 0 {
		t.Errorf("a search returns tracks, not folders: %v", p.Dirs)
	}
	p, _ = lb.Library(web.LibraryQuery{Search: "rock", Dir: "rock/metal", Limit: 100})
	if got := strings.Join(paths(p.Tracks), ","); got != "rock/metal/z.mp3" {
		t.Errorf("search inside a folder = %s", got)
	}
}

func TestLibrary_Tags(t *testing.T) {
	lb := browseBot(t)
	for _, c := range []struct {
		q    web.LibraryQuery
		want string
	}{
		{web.LibraryQuery{Tags: []string{"fav"}}, "rock/metal/z.mp3,rock/x.mp3"},
		{web.LibraryQuery{Tags: []string{"fav", "loud"}}, "rock/metal/z.mp3"},
		{web.LibraryQuery{Tags: []string{"nope"}}, ""},
		{web.LibraryQuery{Tags: []string{"loud"}, Search: "rock"}, "rock/metal/z.mp3"}, // both must hold
		{web.LibraryQuery{Tags: []string{"loud"}, Search: "jazz"}, ""},
	} {
		c.q.Limit = 100
		p, err := lb.Library(c.q)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(paths(p.Tracks), ","); got != c.want {
			t.Errorf("%+v = %q, want %q", c.q, got, c.want)
		}
	}
}

func TestLibrary_Paging(t *testing.T) {
	lb := browseBot(t)
	q := web.LibraryQuery{Search: "rock", Limit: 2}
	p, _ := lb.Library(q)
	if strings.Join(paths(p.Tracks), ",") != "rock/metal/z.mp3,rock/x.mp3" || p.Total != 3 {
		t.Errorf("page 1 = %v total %d", paths(p.Tracks), p.Total)
	}
	q.Offset = 2
	p, _ = lb.Library(q)
	if strings.Join(paths(p.Tracks), ",") != "rock/y.mp3" || p.Total != 3 {
		t.Errorf("page 2 = %v total %d", paths(p.Tracks), p.Total)
	}
	q.Offset = 99
	if p, _ = lb.Library(q); len(p.Tracks) != 0 || p.Total != 3 {
		t.Errorf("past the end = %v total %d, want empty and the real total", paths(p.Tracks), p.Total)
	}
}

func TestLibrary_Unavailable(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, err := b.Library(web.LibraryQuery{}); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if _, err := b.Upload(bg, web.Identity{Name: "x"}, "a.mp3", strings.NewReader("x")); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("upload err = %v, want ErrUnavailable", err)
	}
}

// ---- upload ----

// fakeProbe makes the bot's scanner independent of ffprobe.
func (l *libBot) useFakeProbe() {
	lib := l.libraryRef()
	lib.scanner = library.NewScanner(lib.db, library.Config{
		MusicFolder: l.root,
		Probe: func(abs string) (file.Metadata, error) {
			return file.Metadata{Title: strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)), Artist: "Up", Duration: 61 * time.Second}, nil
		},
	})
}

func TestUpload_StoresAndShowsUpInTheLibrary(t *testing.T) {
	lb := newLibBot(t, 0, func(c *config.Config) { c.Web.MaxUploadBytes = 1 << 20 })
	lb.useFakeProbe()
	track, err := lb.Upload(bg, web.Identity{Subject: "h", Name: "alice"}, "My Song.mp3", strings.NewReader("audio"))
	if err != nil {
		t.Fatal(err)
	}
	if track.Path != "uploads/alice/My Song.mp3" || track.Title != "My Song" || track.Artist != "Up" || track.Duration != 61*time.Second {
		t.Errorf("track = %+v", track)
	}
	if b, err := os.ReadFile(filepath.Join(lb.root, "uploads", "alice", "My Song.mp3")); err != nil || string(b) != "audio" {
		t.Errorf("file on disk = %q, %v", b, err)
	}
	// The new file is queueable by its library path straight away.
	if _, err := lb.Add(bg, web.Source{Type: web.SourceLibrary, Path: track.Path}, 0); err != nil {
		t.Errorf("queueing the upload: %v", err)
	}
	p, _ := lb.Library(web.LibraryQuery{Dir: "uploads/alice", Limit: 10})
	if len(p.Tracks) != 1 || p.Tracks[0].Path != track.Path {
		t.Errorf("browse uploads/alice = %+v", p)
	}
}

func TestUpload_TheSizeLimitComesFromTheConfig(t *testing.T) {
	lb := newLibBot(t, 0, func(c *config.Config) { c.Web.MaxUploadBytes = 8 })
	lb.useFakeProbe()
	if _, err := lb.Upload(bg, web.Identity{Name: "a"}, "ok.mp3", strings.NewReader("12345678")); err != nil {
		t.Errorf("at the limit: %v", err)
	}
	if _, err := lb.Upload(bg, web.Identity{Name: "a"}, "big.mp3", strings.NewReader("123456789")); !errors.Is(err, library.ErrTooLarge) {
		t.Errorf("over the limit: err = %v, want ErrTooLarge", err)
	}
}

// ---- the whole path over HTTP ----

func TestWeb_UploadAndQueueEndToEnd(t *testing.T) {
	port := freePort(t)
	lb := newLibBot(t, 0, func(c *config.Config) {
		c.Web = config.WebConfig{
			Enabled: true, Addr: "127.0.0.1", Port: port, AuthMethod: config.AuthToken,
			AccessAddress: "http://127.0.0.1:" + strconv.Itoa(port),
			UploadEnabled: true, MaxUploadBytes: 1 << 20,
		}
	})
	lb.useFakeProbe()
	b := lb.Bot

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := b.StartWeb(ctx); err != nil {
		t.Fatal(err)
	}
	base := b.cfg.Web.AccessAddress
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	link, _, _ := b.WebLogin(web.Identity{Subject: "h", Name: "alice", Roles: []web.Role{web.RoleUser}})
	if resp, err := c.Get(link); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %v %v", resp, err)
	}

	post := func(name, content string) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		io.WriteString(fw, content)
		mw.Close()
		req, _ := http.NewRequest("POST", base+"/api/v1/library/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	code, out := post("Song.mp3", "audio-bytes")
	track, _ := out["track"].(map[string]any)
	if code != http.StatusCreated || track["path"] != "uploads/alice/Song.mp3" || track["title"] != "Song" {
		t.Fatalf("upload = %d %v", code, out)
	}
	if code, out = post("Song.mp3", "second"); code != http.StatusCreated || out["track"].(map[string]any)["path"] != "uploads/alice/Song (1).mp3" {
		t.Errorf("same name again = %d %v, want a renamed copy", code, out)
	}
	if code, out = post("notes.txt", "nope"); code != http.StatusUnprocessableEntity || out["code"] != "invalid_file" {
		t.Errorf("a text file = %d %v, want 422 invalid_file", code, out)
	}

	// Browse it, then queue it.
	resp, err := c.Get(base + "/api/v1/library?dir=uploads/alice")
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Tracks []struct{ Path string }
		Total  int
	}
	json.NewDecoder(resp.Body).Decode(&page)
	resp.Body.Close()
	if page.Total != 2 {
		t.Errorf("browse uploads/alice = %+v, want 2 tracks", page)
	}
	req, _ := http.NewRequest("POST", base+"/api/v1/queue", strings.NewReader(`{"type":"library","path":"uploads/alice/Song.mp3"}`))
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("queue the upload = %d", resp.StatusCode)
	}
}

// A real scan stores title and artist as the search keywords and adds the path
// words only for files without a title tag. The fixtures above put the path into
// the keywords, which hid that a query matching only the path found nothing.
func TestLibrary_SearchMatchesThePathEvenWhenTheKeywordsDoNot(t *testing.T) {
	lb := browseBot(t)
	lib := lb.libraryRef()
	err := lib.db.Insert(database.MusicRecord{
		ID: file.IDFromPath("metal/heavy/song.mp3"), Type: library.RecordType, Title: "Thunder", Path: "metal/heavy/song.mp3",
		Keywords: "thunder dark band", // what the scan stores: title and artist, no path
		Metadata: map[string]any{"artist": "Dark Band", "duration": float64(200)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		search string
		want   string
	}{
		{"metal", "metal/heavy/song.mp3,rock/metal/z.mp3"}, // a folder name, only in the path (z.mp3 lives in rock/metal)
		{"heavy song", "metal/heavy/song.mp3"},             // words from different path parts
		{"THUNDER", "metal/heavy/song.mp3"},                // the title, any case
		{"dark band", "metal/heavy/song.mp3"},              // the artist
		{"metal thunder", "metal/heavy/song.mp3"},          // path word and title word together
		{"metal nothing", ""},                              // every word must match
	} {
		p, err := lb.Library(web.LibraryQuery{Search: c.search, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(paths(p.Tracks), ","); got != c.want {
			t.Errorf("search %q = %q, want %q", c.search, got, c.want)
		}
	}

	// A tag filter and a path-only search work together, tags matching as whole words.
	if err := lib.db.UpdateTags(file.IDFromPath("metal/heavy/song.mp3"), func([]string) []string { return []string{"HardRock"} }); err != nil {
		t.Fatal(err)
	}
	if p, _ := lb.Library(web.LibraryQuery{Search: "heavy", Tags: []string{"hardrock"}, Limit: 100}); len(p.Tracks) != 1 {
		t.Errorf("tag (any case) + path search = %v", paths(p.Tracks))
	}
	if p, _ := lb.Library(web.LibraryQuery{Search: "heavy", Tags: []string{"rock"}, Limit: 100}); len(p.Tracks) != 0 {
		t.Errorf("a tag must match whole: %v", paths(p.Tracks))
	}
}
