// Command gh-copilot-usage is a gh CLI extension that visualizes GitHub
// Copilot CLI AIC (AI credit) usage as a stacked time-series chart in the
// browser, cross-checked against the monthly billing API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/mazrean/gh-copilot-usage/internal/billing"
	"github.com/mazrean/gh-copilot-usage/internal/server"
	"github.com/mazrean/gh-copilot-usage/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh-copilot-usage:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		dbPath  string
		addr    string
		noOpen  bool
		jsonOut bool
		dim     string
		gran    string
	)
	defaultDB, err := store.DefaultDBPath()
	if err != nil {
		return err
	}

	flag.StringVar(&dbPath, "db", defaultDB, "path to Copilot session-store.db")
	flag.StringVar(&addr, "addr", "127.0.0.1:0", "address to serve the web UI on")
	flag.BoolVar(&noOpen, "no-open", false, "do not open the browser automatically")
	flag.BoolVar(&jsonOut, "json", false, "print aggregated usage as JSON and exit (no server)")
	flag.StringVar(&dim, "dimension", "model", "stacking dimension for --json: model|session")
	flag.StringVar(&gran, "granularity", "day", "time bucket for --json: day|week|month")
	flag.Parse()

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open session-store.db (%s): %w", dbPath, err)
	}
	defer st.Close()

	if jsonOut {
		usage, err := st.Aggregate(context.Background(), store.Dimension(dim), store.Granularity(gran))
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(usage)
	}

	bc, err := billing.NewClient()
	if err != nil {
		slog.Warn("gh billing client unavailable; /api/monthly will report errors", "err", err)
		bc = nil
	}

	handler := server.New(st, bc)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	url := "http://" + ln.Addr().String()
	fmt.Println("gh-copilot-usage serving at", url)

	if !noOpen {
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				slog.Warn("could not open browser automatically", "err", err)
			}
		}()
	}

	srv := &http.Server{Handler: handler}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
