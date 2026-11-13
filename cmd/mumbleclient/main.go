// Command mumbleclient is a very small Mumble client for trying the bot without a desktop
// client: it chats in the terminal, fetches the bot's web login link, and checks that audio
// really reaches a listener. It is a development tool and is not part of the released bot.
//
//	mumbleclient                       chat: type a line to say it in the channel, /quit to leave
//	mumbleclient -login                send !web and print the login link (for scripts: the link only)
//	mumbleclient -listen 5s            listen for audio; exit 0 only if something audible arrived
//
// It does not play sound: -listen reports levels, which is enough to know the bot's audio
// arrives. Use a real Mumble client to hear it.
package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleutil"
	_ "layeh.com/gumble/opus" // registers the Opus decoder, so received audio arrives as PCM
)

// showRaw prints messages untouched instead of as plain text.
var showRaw bool

// shown renders a message for the terminal.
func shown(msg string) string {
	if showRaw {
		return fmt.Sprintf("%q", msg)
	}
	return plainText(msg)
}

// message is a chat message received from the server.
type message struct {
	from    string
	text    string
	private bool
}

func main() {
	os.Exit(run())
}

func run() int {
	addr := flag.String("addr", "127.0.0.1:64738", "Mumble server address")
	name := flag.String("name", "playground-user", "user name to join with")
	channel := flag.String("channel", "", `channel to join, like the bot's own setting (a/b for a sub-channel); default: the root channel`)
	login := flag.Bool("login", false, "ask the bot for its web login link and print it")
	command := flag.String("command", "!web", "the chat command that asks for the link (with -login)")
	listen := flag.Duration("listen", 0, "listen for audio for this long, then report; exit 1 if nothing audible arrived")
	minRMS := flag.Float64("min-rms", 100, "with -listen: the level (RMS of 16-bit samples) that counts as audible")
	within := flag.Duration("wait", 20*time.Second, "how long to keep trying to connect, and to wait for the bot's answer")
	flag.BoolVar(&showRaw, "raw", false, "show chat messages exactly as the server delivered them (HTML and all), for debugging")
	flag.Parse()

	sess, err := connect(*addr, *name, *within)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mumbleclient: %v\n", err)
		return 1
	}
	defer sess.close()
	if err := sess.join(*channel); err != nil {
		fmt.Fprintf(os.Stderr, "mumbleclient: %v\n", err)
		return 1
	}

	switch {
	case *login:
		return runLogin(sess, *command, *within)
	case *listen > 0:
		return runListen(sess, *listen, *minRMS)
	default:
		return runChat(sess)
	}
}

// ---- connection

type session struct {
	client *gumble.Client
	name   string
	texts  chan message
	stats  *audioStats

	mu       sync.Mutex
	onStream func(user string) // called when a user starts sending audio
}

func connect(addr, name string, within time.Duration) (*session, error) {
	s := &session{name: name, texts: make(chan message, 64), stats: newAudioStats()}
	cfg := gumble.NewConfig()
	cfg.Username = name
	cfg.Attach(gumbleutil.Listener{
		TextMessage: func(e *gumble.TextMessageEvent) {
			from := "server"
			if e.Sender != nil {
				from = e.Sender.Name
			}
			select {
			case s.texts <- message{from: from, text: e.Message, private: len(e.Users) > 0}:
			default: // nobody is reading; drop rather than block the connection
			}
		},
	})
	cfg.AttachAudio(s)

	// The server may still be starting; keep trying for a while.
	deadline := time.Now().Add(within)
	for {
		c, err := gumble.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, addr, cfg,
			&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // a local test server with its own certificate
		if err == nil {
			s.client = c
			return s, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("cannot connect to %s: %w", addr, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (s *session) close() {
	if s.client != nil && s.client.State() != gumble.StateDisconnected {
		s.client.Disconnect() //nolint:errcheck
	}
}

// join moves the client into the named channel ("" stays in the root channel) and waits until the
// server has confirmed the move. The bot only hears the channel it is in, so a client has to be in
// the same one to talk to it.
func (s *session) join(name string) error {
	if name == "" {
		return nil
	}
	var target *gumble.Channel
	s.client.Do(func() { target = s.client.Channels.Find(strings.Split(name, "/")...) })
	if target == nil {
		return fmt.Errorf("there is no channel %q on this server", name)
	}
	s.client.Do(func() { s.client.Self.Move(target) })
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		in := false
		s.client.Do(func() { in = s.client.Self.Channel == target })
		if in {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the server did not let me into %q", name)
}

// say sends text to the channel the client is in.
func (s *session) say(text string) {
	s.client.Do(func() { s.client.Self.Channel.Send(text, false) })
}

// OnAudioStream implements gumble.AudioListener: one call per user who starts talking,
// then packets of decoded samples until the stream ends.
func (s *session) OnAudioStream(e *gumble.AudioStreamEvent) {
	user := "someone"
	if e.User != nil {
		user = e.User.Name
	}
	s.mu.Lock()
	notify := s.onStream
	s.mu.Unlock()
	if notify != nil {
		notify(user)
	}
	go func() {
		for p := range e.C {
			s.stats.Add(user, p.AudioBuffer)
		}
	}()
}

// ---- modes

func runLogin(s *session, command string, within time.Duration) int {
	s.say(command)
	deadline := time.After(within)
	var other string // the last private message that held no address, to explain a failure
	for {
		select {
		case m := <-s.texts:
			// The bot answers privately with the link. Other messages are skipped: the
			// notice in the channel, what other people say, and what some servers send
			// on joining (Kallisti replays the channel's history as a private message).
			if !m.private {
				continue
			}
			if u, ok := findURL(m.text); ok {
				fmt.Println(u)
				return 0
			}
			other = fmt.Sprintf("%s said: %s", m.from, shown(m.text))
		case <-deadline:
			if other != "" {
				fmt.Fprintf(os.Stderr, "mumbleclient: no login link within %v. The last private message was: %s\n", within, other)
			} else {
				fmt.Fprintf(os.Stderr, "mumbleclient: no answer to %q within %v. Is the bot in this channel?\n", command, within)
			}
			return 1
		}
	}
}

func runListen(s *session, d time.Duration, minRMS float64) int {
	time.Sleep(d)
	packets, samples, peak, rms, users := s.stats.Snapshot()
	if packets == 0 {
		fmt.Fprintf(os.Stderr, "mumbleclient: heard no audio in %v. Is something playing, in the channel I joined?\n", d)
		return 1
	}
	fmt.Printf("heard %.1fs of audio from %s: level rms=%.0f peak=%d, %d packets\n",
		seconds(samples), strings.Join(users, ", "), rms, peak, packets)
	if rms < minRMS {
		fmt.Fprintf(os.Stderr, "mumbleclient: that is silence (level %.0f, below %.0f)\n", rms, minRMS)
		return 1
	}
	return 0
}

func runChat(s *session) int {
	var channel, users string
	s.client.Do(func() {
		channel = s.client.Self.Channel.Name
		var names []string
		for _, u := range s.client.Users {
			names = append(names, u.Name)
		}
		users = strings.Join(names, ", ")
	})
	fmt.Printf("Joined %q as %s. Here: %s\n", channel, s.name, users)
	fmt.Println("Type a line to say it in the channel (try !web or !help). /quit leaves.")

	// Say when someone starts sending audio, so you know the bot is playing.
	s.mu.Lock()
	s.onStream = func(user string) { fmt.Printf("%s  (hearing audio from %s)\n", stamp(), user) }
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case m := <-s.texts:
				kind := ""
				if m.private {
					kind = " (private)"
				}
				fmt.Printf("%s  %s%s: %s\n", stamp(), m.from, kind, indent(shown(m.text)))
			case <-done:
				return
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	defer close(done)
	for {
		select {
		case line, ok := <-lines:
			line = strings.TrimSpace(line)
			if !ok || line == "/quit" || line == "/exit" {
				return 0
			}
			if line != "" {
				s.say(line)
			}
		case <-sig:
			return 0
		}
	}
}

func stamp() string { return time.Now().Format("15:04:05") }

// indent keeps the continuation lines of a multi-line message under its first line.
func indent(s string) string { return strings.ReplaceAll(s, "\n", "\n          ") }
