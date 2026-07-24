package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	twitterscraper "github.com/jeffrey12cali/twitter-scraper"
)

type CreatorProfileFetcher interface {
	FetchCreatorProfile(ctx context.Context, username string, auth UpdateAuthSecret) (CreatorProfileInput, error)
}

type TwitterProfileFetcher struct {
	fetch func(context.Context, string, UpdateAuthSecret, bool) (CreatorProfileInput, error)
}

func (f TwitterProfileFetcher) FetchCreatorProfile(ctx context.Context, username string, auth UpdateAuthSecret) (CreatorProfileInput, error) {
	username = strings.Trim(strings.TrimSpace(username), "@")
	if username == "" {
		return CreatorProfileInput{}, errors.New("Twitter 作者账号不能为空")
	}
	fetch := f.fetch
	if fetch == nil {
		fetch = fetchCreatorProfileWithAuth
	}
	var lastErr error
	if strings.TrimSpace(auth.CookieHeader) != "" {
		profile, err := fetch(ctx, username, auth, true)
		if err == nil || !isAuthFailure(err) {
			return profile, err
		}
		lastErr = err
	}
	if strings.TrimSpace(auth.AuthToken) != "" || strings.TrimSpace(auth.CT0) != "" {
		profile, err := fetch(ctx, username, auth, false)
		if err == nil || !isAuthFailure(err) {
			return profile, err
		}
		lastErr = err
	}
	profile, err := fetch(ctx, username, UpdateAuthSecret{Proxy: auth.Proxy}, false)
	if err != nil && lastErr != nil {
		return CreatorProfileInput{}, lastErr
	}
	return profile, err
}

func fetchCreatorProfileWithAuth(ctx context.Context, username string, auth UpdateAuthSecret, useCookie bool) (CreatorProfileInput, error) {
	scraper := twitterscraper.New()
	if useCookie {
		cookies, err := parseTwitterCookies(auth.CookieHeader)
		if err != nil {
			return CreatorProfileInput{}, err
		}
		scraper.SetCookies(cookies)
	} else if strings.TrimSpace(auth.AuthToken) != "" || strings.TrimSpace(auth.CT0) != "" {
		scraper.SetAuthToken(twitterscraper.AuthToken{Token: auth.AuthToken, CSRFToken: auth.CT0})
	}
	if proxy := strings.TrimSpace(auth.Proxy); proxy != "" {
		scraper.SetProxy(proxy)
	}
	type result struct {
		profile twitterscraper.Profile
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		profile, err := scraper.GetProfile(username)
		ch <- result{profile: profile, err: err}
	}()
	select {
	case <-ctx.Done():
		return CreatorProfileInput{}, ctx.Err()
	case result := <-ch:
		if result.err != nil {
			return CreatorProfileInput{}, result.err
		}
		profile := result.profile
		return CreatorProfileInput{
			TwitterUserID: profile.UserID,
			Username:      firstProfileNonEmpty(profile.Username, username),
			ProfileURL:    "https://x.com/" + firstProfileNonEmpty(profile.Username, username),
			AvatarURL:     profile.Avatar,
			Bio:           profile.Biography,
		}, nil
	}
}

func RefreshCreatorProfile(ctx context.Context, store *Store, creator Creator, auth UpdateAuthSecret, fetcher CreatorProfileFetcher) error {
	if fetcher == nil {
		fetcher = TwitterProfileFetcher{}
	}
	username, profileURL, ok := canonicalTwitterIdentity(creator.Name, creator.TwitterUsername, creator.TwitterProfileURL)
	if !ok {
		return errors.New("无法解析 Twitter 作者账号")
	}
	profile, err := fetcher.FetchCreatorProfile(ctx, username, auth)
	if err != nil {
		return err
	}
	if strings.TrimSpace(profile.Username) == "" {
		profile.Username = username
	}
	if strings.TrimSpace(profile.ProfileURL) == "" {
		profile.ProfileURL = profileURL
	}
	if strings.TrimSpace(profile.AvatarURL) != "" {
		if avatarFile, err := downloadCreatorAvatar(ctx, store, creator.ID, profile.AvatarURL, auth); err == nil {
			profile.AvatarFile = avatarFile
		}
	}
	return store.UpdateCreatorProfile(creator.ID, profile)
}

func (s *Store) CreatorAvatarFile(id int64) (string, error) {
	var avatarFile string
	err := s.db.QueryRow(`SELECT avatar_file FROM creators WHERE id=?`, id).Scan(&avatarFile)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(avatarFile) == "" {
		return "", os.ErrNotExist
	}
	return avatarFile, nil
}

func (s *Store) avatarDir() string {
	base := filepath.Dir(s.path)
	if s.path == "" || s.path == ":memory:" || base == "." {
		base = os.TempDir()
	}
	return filepath.Join(base, "localtwitter-assets", "avatars")
}

func downloadCreatorAvatar(ctx context.Context, store *Store, creatorID int64, avatarURL string, auth UpdateAuthSecret) (string, error) {
	if err := os.MkdirAll(store.avatarDir(), 0o755); err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(strings.Split(avatarURL, "?")[0]))
	if ext == "" || len(ext) > 5 {
		ext = ".jpg"
	}
	target := filepath.Join(store.avatarDir(), strconv.FormatInt(creatorID, 10)+ext)
	tmp := target + ".tmp"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second, Transport: avatarTransport(auth.Proxy)}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("头像下载失败: " + resp.Status)
	}
	file, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return target, nil
}

func avatarTransport(proxyRaw string) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(proxyRaw) != "" {
		if proxyURL, err := url.Parse(proxyRaw); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}
	return transport
}

func firstProfileNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "401") || strings.Contains(message, "403") || strings.Contains(message, "unauthorized") || strings.Contains(message, "authenticate")
}
