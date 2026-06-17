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
	"time"

	"deaddrop/internal/hub"
	"deaddrop/internal/ws"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getenv("PORT", "8080")
	maxFrame, err := strconv.ParseInt(getenv("MAX_FRAME_BYTES", "10000000"), 10, 64)
	if err != nil {
		log.Fatalf("invalid MAX_FRAME_BYTES: %v", err)
	}

	h := hub.New()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.Serve(h, maxFrame, w, r)
	})
	mux.HandleFunc("/cli", cliScriptHandler)
	mux.HandleFunc("/cli/bin/", cliBinHandler)
	mux.Handle("/", spaHandler())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("DeadDrop listening on :%s (max frame %d bytes)", port, maxFrame)
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
