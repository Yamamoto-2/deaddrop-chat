// Command cli is the DeadDrop terminal client: a tiny static binary that speaks
// the same WebSocket + E2EE as the web app. Delivered fileless via
// `curl https://<host>/cli | sh`. The default UI is a full-screen TUI (tui.go);
// `-send` is a headless one-shot used for scripted interop tests.
package main

import (
	crand "crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

// Same palette as the web client, for nick colors.
var palette = []string{
	"#5fd7a7", "#5fafff", "#ff87d7", "#ffd75f",
	"#ff8787", "#af87ff", "#5fd7d7", "#ffaf5f",
	"#87d75f", "#d7d787", "#ff6fb5", "#9d9dff",
}

func main() {
	send := flag.String("send", "", "headless: send one message, listen briefly, then exit (testing)")
	listen := flag.Duration("listen", 3*time.Second, "headless listen duration")
	flag.Parse()

	server := envOr("DD_SERVER", "ws://127.0.0.1:8080/ws")
	room, pass, nick := config(flag.Args())

	if *send != "" {
		runHeadless(server, room, pass, nick, *send, *listen)
		return
	}
	runTUI(server, room, pass, nick)
}

func runHeadless(server, room, pass, nick, text string, listen time.Duration) {
	if room == "" {
		fmt.Fprintln(os.Stderr, "no room specified")
		os.Exit(1)
	}
	if nick == "" {
		nick = "anon"
	}
	c, err := Dial(server, room, pass)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect failed:", err)
		os.Exit(1)
	}
	defer c.Close()

	ch := make(chan Incoming, 16)
	go c.ReadLoop(ch)
	go func() {
		for in := range ch {
			switch in.Kind {
			case "msg":
				printMsg(in.Msg)
			case "presence":
				fmt.Printf("\033[2m◍ %d online\033[0m\n", in.N)
			}
		}
	}()

	color := palette[randInt(len(palette))]
	_ = c.Send(payload{Nick: nick, Color: color, Ts: nowMs(), Text: text})
	fmt.Print("(sent) ")
	printMsg(payload{Nick: nick, Color: color, Text: text})
	time.Sleep(listen)
}

// --- helpers ---

func printMsg(p payload) {
	r, g, b := hexRGB(p.Color)
	fmt.Printf("\033[38;2;%d;%d;%dm%s\033[0m  %s\n", r, g, b, p.Nick, p.Text)
}

func config(args []string) (room, pass, nick string) {
	room = os.Getenv("DD_ROOM")
	pass = os.Getenv("DD_PASS")
	nick = os.Getenv("DD_NICK")
	if room == "" && len(args) >= 1 {
		rp := args[0]
		if i := strings.Index(rp, ":"); i >= 0 {
			room, pass = rp[:i], rp[i+1:]
		} else {
			room = rp
		}
	}
	if nick == "" && len(args) >= 2 {
		nick = args[1]
	}
	return room, pass, nick
}

func hexRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 215, 224, 218
	}
	var r, g, b int
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func nowMs() int64 { return time.Now().UnixMilli() }

func randInt(n int) int {
	x, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(x.Int64())
}
