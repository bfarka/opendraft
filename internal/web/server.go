package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/bfarka/opendraft/internal/control"
)

//go:embed static/*
var staticFS embed.FS

// NewHandler returns an http.Handler serving the embedded WebUI and JSON/SSE API.
func NewHandler(bridge *Bridge) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, bridge.Snapshot())
	})

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch, cancel := bridge.Subscribe()
		defer cancel()

		if err := writeSSE(w, flusher, bridge.Snapshot()); err != nil {
			return
		}

		heartbeat := time.NewTicker(2 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case st, ok := <-ch:
				if !ok {
					return
				}
				if err := writeSSE(w, flusher, st); err != nil {
					return
				}
			case <-heartbeat.C:
				if err := writeSSE(w, flusher, bridge.Snapshot()); err != nil {
					return
				}
			}
		}
	})

	mux.HandleFunc("POST /api/start", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, bridge.Start())
	})

	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, bridge.Stop())
	})

	mux.HandleFunc("POST /api/mode", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		mode := control.Mode(strings.ToUpper(strings.TrimSpace(body.Mode)))
		st, err := bridge.SetMode(mode)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/speed", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Speed int `json:"speed"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := bridge.SetManualSpeed(body.Speed)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/curve", func(w http.ResponseWriter, r *http.Request) {
		var body CurveConfig
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := bridge.UpdateCurve(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/scan", func(w http.ResponseWriter, r *http.Request) {
		st, err := bridge.StartScan()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/scan/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, bridge.StopScan())
	})

	mux.HandleFunc("POST /api/devices/hr", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := bridge.ConnectHR(strings.TrimSpace(body.ID))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	mux.HandleFunc("POST /api/devices/fan", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := bridge.SelectFan(strings.TrimSpace(body.ID))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, st)
	})

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(fmt.Sprintf("embed static fs: %v", err))
	}
	mux.Handle("GET /", http.FileServer(http.FS(sub)))

	return mux
}

// StartServer binds addr, serves the WebUI until ctx is cancelled, and returns the bound address.
func StartServer(ctx context.Context, addr string, bridge *Bridge) (string, <-chan error, error) {
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return "", nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           NewHandler(bridge),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	return ln.Addr().String(), errCh, nil
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, st State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
