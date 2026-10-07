package controller

import (
	"crypto/hmac"
	"net/http"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
)

// OAuth state authenticates the flow, while this host-only browser cookie
// authenticates its initiator. Possessing one's own state and provider code
// must not allow an attacker to log another browser into the attacker's account.
// Lax admits the provider's top-level GET redirect. Per-flow names preserve
// independent popups; __Host forbids parent-domain cookie injection.
func oauthBrowserBinding(state string) (string, string) {
	value := common.GenerateHMACWithKey([]byte("further-api/oauth-browser/v1:"+common.SessionSecret), state)
	prefix := "further_api_oauth_"
	if common.SessionCookieSecure {
		prefix = "__Host-" + prefix
	}
	return prefix + value[:24], value
}

func writeOAuthBrowserBinding(c *gin.Context, state string, expiresAt int64) {
	name, value := oauthBrowserBinding(state)
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: common.SessionCookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Unix(expiresAt, 0), MaxAge: max(int(expiresAt-time.Now().Unix()), 1)})
}

func validateOAuthBrowserBinding(c *gin.Context, state string) bool {
	name, expected := oauthBrowserBinding(state)
	value, err := c.Cookie(name)
	return err == nil && hmac.Equal([]byte(value), []byte(expected))
}

func clearOAuthBrowserBinding(c *gin.Context, state string) {
	name, _ := oauthBrowserBinding(state)
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: common.SessionCookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0), MaxAge: -1})
}
