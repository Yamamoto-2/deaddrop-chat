// Command deaddrop is an anonymous, ephemeral, end-to-end-encrypted chat relay.
// The server is a "dead drop": it groups sockets by an opaque routing id and
// fans out ciphertext. It never sees plaintext, room names, or nicknames.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"deaddrop/internal/hub"
	"deaddrop/internal/ws"
)

// version is stamped at release time via -ldflags "-X main.version=...".
var version = "dev"

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	return def
}

// splitOrigins turns "a,b" into ["a","b"]; "*" (the default) stays permissive.
func splitOrigins(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	port := getenv("PORT", "7337")
	maxFrame, err := strconv.ParseInt(getenv("MAX_FRAME_BYTES", "10000000"), 10, 64)
	if err != nil {
		log.Fatalf("invalid MAX_FRAME_BYTES: %v", err)
	}

	h := hub.New()
	cfg := ws.Config{
		MaxFrame:       maxFrame,
		IdleTimeout:    time.Duration(getenvInt("IDLE_TIMEOUT_SEC", 90)) * time.Second,
		MaxConnsPerIP:  getenvInt("MAX_CONNS_PER_IP", 64),
		MsgBurst:       float64(getenvInt("MSG_BURST", 40)),
		MsgRate:        float64(getenvInt("MSG_RATE", 20)),
		TrustProxy:     getenvBool("TRUST_PROXY", false),
		AllowedOrigins: splitOrigins(getenv("ALLOWED_ORIGINS", "*")),
	}
	srvWS := ws.NewServer(h, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srvWS.Serve)
	mux.HandleFunc("/cli", cliScriptHandler)
	mux.HandleFunc("/cli/bin/", cliBinHandler)
	mux.Handle("/", spaHandler())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("DeadDrop %s listening on :%s (max frame %d bytes, idle %s, %d conns/ip, %g msg/s)",
			version, port, maxFrame, cfg.IdleTimeout, cfg.MaxConnsPerIP, cfg.MsgRate)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Println("shutdown complete")
}
