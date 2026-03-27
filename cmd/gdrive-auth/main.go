// Command gdrive-auth performs a one-time OAuth2 browser flow and saves the
// resulting token (including refresh_token) to token.json. Run this once on
// any machine with a browser, then copy credentials.json and token.json to
// the server.
//
// Usage:
//
//	go run ./cmd/gdrive-auth -credentials credentials.json -token token.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

func main() {
	credFile := flag.String("credentials", "credentials.json", "path to OAuth2 credentials JSON")
	tokenOut := flag.String("token", "token.json", "where to write the saved token")
	flag.Parse()

	data, err := os.ReadFile(*credFile)
	if err != nil {
		log.Fatalf("read %s: %v", *credFile, err)
	}

	cfg, err := google.ConfigFromJSON(data, drive.DriveReadonlyScope)
	if err != nil {
		log.Fatalf("parse config: %v", err)
	}

	tok, err := tokenFromWeb(cfg)
	if err != nil {
		log.Fatalf("auth flow: %v", err)
	}

	out, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		log.Fatalf("marshal token: %v", err)
	}
	if err := os.WriteFile(*tokenOut, out, 0600); err != nil {
		log.Fatalf("write %s: %v", *tokenOut, err)
	}
	fmt.Printf("Token saved to %s — copy both files to the server.\n", *tokenOut)
}

// tokenFromWeb runs a local redirect server, opens the browser auth URL, and
// waits for the authorization code. It then exchanges the code for a token.
func tokenFromWeb(cfg *oauth2.Config) (*oauth2.Token, error) {
	// Use localhost redirect so the installed-app flow works without a public server.
	cfg.RedirectURL = "http://localhost:8085/callback"

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8085"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Authorization complete — you can close this tab.")
		codeCh <- code
	})
	go func() { _ = srv.ListenAndServe() }()
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	// AccessTypeOffline ensures we get a refresh_token.
	authURL := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("\nOpen this URL in your browser:\n\n  %s\n\nWaiting for redirect on http://localhost:8085/callback …\n", authURL)

	code := <-codeCh
	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh_token received — revoke app access at myaccount.google.com/permissions and try again")
	}
	return tok, nil
}
