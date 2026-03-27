package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// DriveAuthConfig holds paths required for the Google Drive OAuth2 flow.
type DriveAuthConfig struct {
	CredFile  string // path to credentials.json (OAuth2 installed-app)
	TokenFile string // path where token.json will be written
}

// SetDriveAuth registers the Drive credentials so the /gdrive_auth command works.
func (t *TgBot) SetDriveAuth(cfg DriveAuthConfig) {
	t.driveAuthCfg = &cfg
}

// handleGDriveAuth is an admin-only command that initiates the Google Drive OAuth2 flow.
// It generates an authorization URL and sends it to the admin. The admin opens the
// URL, signs in, and pastes the code from the redirect back to the bot.
func (t *TgBot) handleGDriveAuth(b *tgbotapi.Bot, ctx *ext.Context) error {
	if !t.isAdminID(ctx.EffectiveUser.Id) {
		return nil
	}
	if t.driveAuthCfg == nil {
		_, _ = ctx.EffectiveMessage.Reply(b, "Drive auth not configured.", nil)
		return nil
	}

	data, err := os.ReadFile(t.driveAuthCfg.CredFile)
	if err != nil {
		_, _ = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Cannot read credentials file: %v", err), nil)
		return nil
	}

	cfg, err := google.ConfigFromJSON(data, drive.DriveReadonlyScope)
	if err != nil {
		_, _ = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Invalid credentials JSON: %v", err), nil)
		return nil
	}

	// AccessTypeOffline ensures we receive a refresh_token for long-term server use.
	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Park the oauth2.Config keyed by admin ID — the next text message from
	// this admin will be treated as the authorization code.
	t.pendingDriveAuth.Store(ctx.EffectiveUser.Id, cfg)

	msg := "Google Drive auth — follow these steps:\n\n" +
		"1. Open this URL in your browser:\n" + authURL + "\n\n" +
		"2. Sign in with the Google account that has access to the Drive folder.\n\n" +
		"3. After clicking Allow, the browser redirects to http://localhost (page won't load — that's OK).\n\n" +
		"4. Copy only the code value from the URL bar:\n" +
		"http://localhost/?code=THIS_PART&scope=...\n\n" +
		"5. Send me just that code."

	_, _ = ctx.EffectiveMessage.Reply(b, msg, nil)
	return nil
}

// handleDriveAuthCode exchanges the authorization code pasted by the admin and saves the token.
func (t *TgBot) handleDriveAuthCode(b *tgbotapi.Bot, ctx *ext.Context) error {
	code := ctx.EffectiveMessage.Text
	raw, _ := t.pendingDriveAuth.LoadAndDelete(ctx.EffectiveUser.Id)
	cfg := raw.(*oauth2.Config)

	tok, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		_, _ = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Failed to exchange code: %v\n\nRun /gdrive_auth again.", err), nil)
		return nil
	}
	if tok.RefreshToken == "" {
		_, _ = ctx.EffectiveMessage.Reply(b,
			"No refresh_token received.\n\n"+
				"Revoke app access at https://myaccount.google.com/permissions then run /gdrive_auth again.",
			nil)
		return nil
	}

	out, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		_, _ = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Failed to encode token: %v", err), nil)
		return nil
	}
	if err := os.WriteFile(t.driveAuthCfg.TokenFile, out, 0600); err != nil {
		_, _ = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Failed to save token: %v", err), nil)
		return nil
	}

	_, _ = ctx.EffectiveMessage.Reply(b,
		fmt.Sprintf("Drive auth complete. Token saved to %s.\n\nRestart the bot to activate Drive access.", t.driveAuthCfg.TokenFile),
		nil)
	return nil
}

// isAdminID returns true if id is in the admin list.
func (t *TgBot) isAdminID(id int64) bool {
	for _, a := range t.adminIds {
		if a == id {
			return true
		}
	}
	return false
}
