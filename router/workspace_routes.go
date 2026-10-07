package router

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Workspace documents own feature navigation. Serving the selected document
// directly avoids a public-page render followed by a second document request.
func workspaceDocumentTarget(requestURL *url.URL) (document, destination string) {
	pathname := strings.TrimSuffix(requestURL.Path, "/")
	panels := map[string]string{
		"/system-settings/site":       "/admin/#settings",
		"/system-settings/auth":       "/admin/#settings",
		"/system-settings/security":   "/admin/#settings",
		"/system-settings/models":     "/admin/#settings",
		"/system-settings/billing":    "/admin/#settings",
		"/system-settings/content":    "/admin/#settings",
		"/system-settings/operations": "/admin/#settings",
		"/keys":                       "/console/#tokens", "/chat": "/console/#tokens", "/chat2link": "/console/#tokens",
		"/profile": "/console/#profile", "/security": "/console/#profile", "/wallet": "/console/#wallet",
		"/activities": "/console/#activities", "/activity": "/console/#activities", "/benefits": "/console/#activities",
		"/checkin": "/console/#activities", "/profile/checkin": "/console/#activities",
		"/channels": "/admin/#channels", "/users": "/admin/#users", "/models": "/admin/#models",
		"/models/metadata": "/admin/#models", "/models/deployments": "/admin/#models",
		"/redemption-codes": "/admin/#redemptions", "/subscriptions": "/admin/#subscriptions/paid",
		"/risk-control": "/admin/#risk", "/system-settings": "/admin/#settings",
		"/dashboard/models": "/admin/#overview/analysis", "/system-info": "/admin/#settings/system-info",
		"/usage-logs": "/admin/#logs", "/usage-logs/common": "/admin/#logs",
		"/system-settings/site/header-navigation":   "/admin/#settings",
		"/system-settings/site/sidebar-modules":     "/admin/#settings",
		"/system-settings/site/system-info":         "/admin/#settings/site-system-info",
		"/system-settings/content/announcements":    "/admin/#settings/announcements",
		"/system-settings/content/api-info":         "/admin/#settings/site-system-info",
		"/system-settings/content/faq":              "/admin/#settings/site-system-info",
		"/system-settings/content/uptime-kuma":      "/admin/#settings/system-info",
		"/system-settings/request-policies":         "/admin/#settings/routing",
		"/system-settings/site/notice":              "/admin/#settings/announcements",
		"/system-settings/request-policies/routing": "/admin/#settings/routing",
		"/system-settings/billing/payment":          "/admin/#settings/payment",
		"/system-settings/billing/model-pricing":    "/admin/#models",
		"/system-settings/billing/group-pricing":    "/admin/#models",
		"/system-settings/billing/checkin":          "/admin/#activities",
	}
	if target, found := panels[pathname]; found {
		destination = target
		if requestURL.RawQuery != "" {
			base, fragment, _ := strings.Cut(target, "#")
			destination = base + "?" + requestURL.RawQuery + "#" + fragment
		}
	} else if festival, found := strings.CutPrefix(pathname, "/activities/"); found {
		switch festival {
		case "duanwu", "qixi", "mid_autumn", "chongyang", "new_year_eve", "national_day":
			destination = "/console/#activities/" + festival
		}
	} else {
		root, _, _ := strings.Cut(strings.TrimPrefix(pathname, "/"), "/")
		canonical := pathname
		if requestURL.RawQuery != "" {
			canonical += "?" + requestURL.RawQuery
		}
		switch root {
		case "playground", "dashboard", "usage-logs":
			destination = "/admin/#workspace/" + url.PathEscape(canonical)
		case "models", "system-settings", "system-info", "task-plugins":
			destination = "/admin/#workspace/" + url.PathEscape(canonical)
		case "chat", "chat2link":
			destination = "/console/#tokens"
		}
	}
	if strings.HasPrefix(destination, "/admin/") {
		return "admin/index.html", destination
	}
	if strings.HasPrefix(destination, "/console/") {
		return "console/index.html", destination
	}
	return "", ""
}

func workspaceNavigationScript(destination string) []byte {
	encoded, _ := json.Marshal(destination)
	return []byte("<script>history.replaceState(history.state,''," + string(encoded) + ");</script>")
}
