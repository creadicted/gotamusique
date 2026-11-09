package bot

import "testing"

func TestFormatTrackAnnouncement(t *testing.T) {
	if got := formatTrackAnnouncement("Artist - Song"); got != "Now playing: Artist - Song" {
		t.Errorf("got %q", got)
	}
	// Titles come from the station: markup must not reach the chat unescaped.
	if got := formatTrackAnnouncement("<b>x</b> & y"); got != "Now playing: &lt;b&gt;x&lt;/b&gt; &amp; y" {
		t.Errorf("got %q", got)
	}
}
