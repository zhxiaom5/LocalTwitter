package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var twitterUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

func normalizeTwitterProfileURL(_ context.Context, raw string, _ *http.Client) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("url is required")
	}
	if !strings.Contains(raw, "://") && (strings.HasPrefix(strings.ToLower(raw), "x.com/") || strings.HasPrefix(strings.ToLower(raw), "twitter.com/") || strings.HasPrefix(strings.ToLower(raw), "www.x.com/") || strings.HasPrefix(strings.ToLower(raw), "www.twitter.com/")) {
		raw = "https://" + raw
	}
	if !strings.Contains(raw, "://") && !strings.Contains(raw, "/") {
		username := strings.TrimPrefix(raw, "@")
		if !twitterUsernamePattern.MatchString(username) {
			return "", "", errors.New("invalid twitter username")
		}
		return "https://x.com/" + username, username, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", errors.New("invalid url")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "x.com" && host != "twitter.com" && host != "www.x.com" && host != "www.twitter.com" {
		return "", "", errors.New("unsupported twitter url")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", errors.New("missing twitter username")
	}
	username, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", errors.New("invalid twitter username")
	}
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" || strings.EqualFold(username, "i") || strings.EqualFold(username, "home") {
		return "", "", errors.New("missing twitter username")
	}
	if !twitterUsernamePattern.MatchString(username) {
		return "", "", errors.New("invalid twitter username")
	}
	return "https://x.com/" + username, username, nil
}

func canonicalTwitterIdentity(name, twitterUsername, profileURL string) (string, string, bool) {
	if username, ok := canonicalTwitterUsername(twitterUsername); ok {
		return username, "https://x.com/" + username, true
	}
	if normalized, username, err := normalizeTwitterProfileURL(context.Background(), profileURL, nil); err == nil {
		return username, normalized, true
	}
	if username, ok := canonicalTwitterUsername(name); ok {
		return username, "https://x.com/" + username, true
	}
	return "", "", false
}

func canonicalTwitterUsername(raw string) (string, bool) {
	username := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	if username == "" || !twitterUsernamePattern.MatchString(username) {
		return "", false
	}
	return username, true
}

func enrichCreatorIdentity(name string, identity CreatorIdentity) CreatorIdentity {
	username, profileURL, ok := canonicalTwitterIdentity(name, identity.TwitterUsername, identity.ProfileURL)
	if !ok {
		return identity
	}
	identity.TwitterUsername = username
	identity.ProfileURL = profileURL
	return identity
}
