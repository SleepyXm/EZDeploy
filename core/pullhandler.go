package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const pullHandlerTimeout = 10 * time.Second

// RegisterPullHandler mounts POST /ezdeploy-pull on mux.
// Nginx exposes this path through the normal server block.
func RegisterPullHandler(mux *http.ServeMux) {
	mux.HandleFunc("/ezdeploy-pull", handlePull)
}

// handlePull processes POST /ezdeploy-pull.
//
// Request body (JSON):
//
//	{ "token": "<payloadB64>.<sigB64>" }
//
// The token is self-describing — the project name is embedded and signed
// inside the payload. The server does not need a project name in the request.
func handlePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 512)).Decode(&req); err != nil || req.Token == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// VerifyToken does everything: parse payload, look up signing key,
	// verify Ed25519 signature, check expiry. No stored token to compare.
	payload, err := VerifyToken(req.Token)
	if err != nil {
		// Same response for every failure mode — don't leak which part failed.
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"accepted","project":%q}`, payload.Project)

	go func() {
		if err := PullAndRedeploy(payload.Project); err != nil {
			fmt.Printf("[!] Pull-triggered redeploy failed for %s: %v\n", payload.Project, err)
		}
	}()
}
