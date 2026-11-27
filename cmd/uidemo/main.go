// Command uidemo serves the gotamusique web interface with a fake bot, for trying out
// and developing the interface without a Mumble server, ffmpeg or a music folder:
//
//	make ui-demo        # then open http://127.0.0.1:8181
//
// It is a development tool. It is not built into the released bot.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/web"
)

func main() {
	os.Exit(run())
}

func run() int {
	addr := flag.String("addr", "127.0.0.1:8181", "address to listen on")
	login := flag.Bool("login", false, "require a login: prints the one-time link instead of skipping the login")
	flag.Parse()

	auth := web.NewAuth(web.AuthOptions{})
	method := config.AuthNone
	if *login {
		method = config.AuthToken
	}
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uidemo: -addr %q: %v\n", *addr, err)
		return 2
	}
	public := "http://" + net.JoinHostPort(host, port)
	srv := web.NewServer(newDemo(), auth, web.Options{
		Addr:                   *addr,
		AccessAddress:          public,
		AuthMethod:             method,
		Log:                    slog.Default(),
		UploadEnabled:          true,
		AllowUploadWithoutAuth: true, // nothing is stored: the demo bot discards uploads
		MaxUploadBytes:         5 << 20,
	})

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uidemo: %v\n", err)
		return 1
	}
	fmt.Printf("gotamusique interface demo: %s\n", public)
	fmt.Println("A fake bot with made-up music; nothing is played or stored. Ctrl+C to stop.")
	if *login {
		tok, err := auth.IssueToken(web.Identity{Subject: "demo", Name: "demo", Roles: []web.Role{web.RoleUser}})
		if err != nil {
			fmt.Fprintf(os.Stderr, "uidemo: %v\n", err)
			return 1
		}
		fmt.Printf("Log in once with: %s\n", web.LoginURL(public, tok))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Serve(ctx, ln); err != nil {
		fmt.Fprintf(os.Stderr, "uidemo: %v\n", err)
		return 1
	}
	return 0
}
