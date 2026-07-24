package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	twitterscraper "github.com/jeffrey12cali/twitter-scraper"
)

type TwitterDownloader struct {
	HTTPClient   *http.Client
	RetrySleeper func(context.Context, time.Duration) error
}

const (
	videoDownloadMaxAttempts       = 3
	videoDownloadIdleTimeout       = 45 * time.Second
	videoDownloadMaxRetryAfterWait = 60 * time.Second
)

var videoDownloadBackoffs = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

type videoDownloadRetryError struct {
	Attempts int
	Last     error
}

func (e videoDownloadRetryError) Error() string {
	if e.Last == nil {
		return fmt.Sprintf("视频下载失败，已自动重试 %d 次", e.Attempts)
	}
	return fmt.Sprintf("视频下载失败，已自动重试 %d 次: %v", e.Attempts, e.Last)
}

func (e videoDownloadRetryError) Unwrap() error {
	return e.Last
}

type httpStatusDownloadError struct {
	StatusCode int
	Status     string
	RetryAfter time.Duration
}

func (e httpStatusDownloadError) Error() string {
	return "下载失败，HTTP " + e.Status
}

type idleReadTimeoutError struct {
	Timeout time.Duration
}

func (e idleReadTimeoutError) Error() string {
	return "视频下载读取超时，超过 " + e.Timeout.String() + " 没有收到新数据"
}

func (d TwitterDownloader) Ready(ctx context.Context) error {
	return ctx.Err()
}

func (d TwitterDownloader) ValidateAuth(ctx context.Context, auth UpdateAuthSecret) error {
	if err := validateTwitterAuth(auth); err != nil {
		return err
	}
	scraper, err := newTwitterScraper(auth)
	if err != nil {
		return err
	}
	if !scraper.IsLoggedIn() {
		return twitterLoginUnavailableError(auth)
	}
	return ctx.Err()
}

func (d TwitterDownloader) UpdateCreator(ctx context.Context, task DownloadTask) (DownloadResult, error) {
	if err := validateTwitterAuth(task.Auth); err != nil {
		return DownloadResult{}, err
	}
	username := strings.TrimPrefix(strings.TrimSpace(task.Username), "@")
	if username == "" {
		return DownloadResult{}, errors.New("Twitter 作者账号不能为空")
	}
	if strings.TrimSpace(task.OutputDir) == "" && strings.TrimSpace(task.VideoDir) == "" {
		return DownloadResult{}, errors.New("下载输出目录不能为空")
	}
	videoDir := strings.TrimSpace(task.VideoDir)
	if videoDir == "" {
		videoDir = filepath.Join(task.OutputDir, username, "video")
	}
	if err := ensureWritableDirectory(videoDir); err != nil {
		return DownloadResult{}, err
	}

	scraper, err := newTwitterScraper(task.Auth)
	if err != nil {
		return DownloadResult{}, err
	}
	if !scraper.IsLoggedIn() {
		return DownloadResult{}, twitterLoginUnavailableError(task.Auth)
	}
	limit := task.Auth.FetchLimit
	if limit <= 0 {
		limit = 300
	}
	tweets := scraper.GetMediaTweets(ctx, username, limit)
	added := 0
	progress := DownloadProgress{}
	report := func(message string) {
		progress.LastMessage = message
		if task.ReportProgress != nil {
			task.ReportProgress(progress)
		}
	}
	for result := range tweets {
		if result == nil {
			continue
		}
		progress.TotalTweets++
		progress.ProcessedTweets++
		if result.Error != nil {
			report("推文列表读取失败")
			return DownloadResult{AddedWorks: added}, result.Error
		}
		if result.IsRetweet && !task.Auth.IncludeRetweets {
			progress.SkippedTweets++
			report("已跳过转推")
			continue
		}
		if len(result.Videos) == 0 {
			progress.SkippedTweets++
			report("已跳过无视频推文")
			continue
		}
		downloadedThisTweet := 0
		for _, video := range result.Videos {
			videoURL := strings.Split(video.URL, "?")[0]
			if strings.TrimSpace(videoURL) == "" {
				continue
			}
			videoName := TWMDMediaFileName(time.Unix(result.Timestamp, 0), videoURL, result.Text, filepath.Ext(videoURL))
			videoPath := filepath.Join(videoDir, videoName)
			if fileExists(videoPath) || fileExists(filepath.Join(videoDir, originalTWMDName(videoName))) {
				continue
			}
			progress.DownloadSpeedBps = 0
			if err := d.downloadWithRetry(ctx, videoURL, videoPath, func(bytesPerSecond int64) {
				progress.DownloadSpeedBps = bytesPerSecond
				report("正在下载视频")
			}, func(message string) {
				progress.DownloadSpeedBps = 0
				report(message)
			}); err != nil {
				report("视频下载失败")
				return DownloadResult{AddedWorks: added}, err
			}
			progress.DownloadSpeedBps = 0
			_ = d.downloadThumbnail(ctx, result, video, videoURL, videoDir)
			_ = writeTWMDJSON(result, videoURL, videoDir)
			_ = writeTWMDNFO(result, videoURL, videoDir)
			_ = writeTWMDASS(result, videoURL, videoDir)
			added++
			downloadedThisTweet++
			progress.DownloadedVideos++
		}
		if downloadedThisTweet == 0 {
			progress.SkippedTweets++
			report("已跳过已有或无可用视频 URL 的推文")
			continue
		}
		report("已下载视频 " + strconv.Itoa(progress.DownloadedVideos))
	}
	report("更新完成")
	return DownloadResult{AddedWorks: added}, nil
}

func parseTwitterCookies(raw string) ([]*http.Cookie, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, errors.New("Cookie 不能为空")
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		var exported []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if strings.HasPrefix(value, "{") {
			var one struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}
			if err := json.Unmarshal([]byte(value), &one); err != nil {
				return nil, errors.New("Cookie JSON 无法解析")
			}
			exported = []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}{one}
		} else if err := json.Unmarshal([]byte(value), &exported); err != nil {
			return nil, errors.New("Cookie JSON 无法解析")
		}
		cookies := make([]*http.Cookie, 0, len(exported))
		for _, item := range exported {
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			cookies = append(cookies, &http.Cookie{Name: strings.TrimSpace(item.Name), Value: item.Value, Path: "/", Domain: "x.com"})
		}
		if len(cookies) == 0 {
			return nil, errors.New("Cookie 中没有可用条目")
		}
		return cookies, nil
	}
	cookies := []*http.Cookie{}
	for _, part := range strings.Split(value, ";") {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		cookies = append(cookies, &http.Cookie{Name: strings.TrimSpace(key), Value: val, Path: "/", Domain: "x.com"})
	}
	if len(cookies) == 0 {
		return nil, errors.New("Cookie 中没有可用条目")
	}
	return cookies, nil
}

func newTwitterScraper(auth UpdateAuthSecret) (*twitterscraper.Scraper, error) {
	scraper := twitterscraper.New()
	if strings.TrimSpace(auth.CookieHeader) != "" {
		cookies, err := parseTwitterCookies(auth.CookieHeader)
		if err != nil {
			return nil, err
		}
		scraper.SetCookies(cookies)
	} else {
		scraper.SetAuthToken(twitterscraper.AuthToken{Token: auth.AuthToken, CSRFToken: auth.CT0})
	}
	if strings.TrimSpace(auth.Proxy) != "" {
		scraper.SetProxy(auth.Proxy)
	}
	return scraper, nil
}

func twitterLoginUnavailableError(auth UpdateAuthSecret) error {
	if strings.TrimSpace(auth.Proxy) != "" {
		return errors.New("Twitter 登录配置不可用或代理不可用，请检查完整 Cookie 和代理地址")
	}
	return errors.New("Twitter 登录配置不可用，请更新 auth_token/ct0 或完整 Cookie")
}

func (d TwitterDownloader) client() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}

func (d TwitterDownloader) download(ctx context.Context, source string, target string, reportSpeed func(int64)) error {
	return d.downloadWithRetry(ctx, source, target, reportSpeed, nil)
}

func (d TwitterDownloader) downloadWithRetry(ctx context.Context, source string, target string, reportSpeed func(int64), reportMessage func(string)) error {
	var lastErr error
	for attempt := 1; attempt <= videoDownloadMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = os.Remove(target + ".tmp")
		err := d.downloadOnce(ctx, source, target, reportSpeed)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryableDownloadError(err) {
			return err
		}
		if attempt == videoDownloadMaxAttempts {
			break
		}
		delay := retryDelayForDownloadError(err, attempt)
		if reportMessage != nil {
			reportMessage(fmt.Sprintf("视频下载失败，%s 后重试（%d/%d）", formatRetryDelay(delay), attempt+1, videoDownloadMaxAttempts))
		}
		if err := d.sleep(ctx, delay); err != nil {
			return err
		}
	}
	return videoDownloadRetryError{Attempts: videoDownloadMaxAttempts, Last: lastErr}
}

func (d TwitterDownloader) downloadOnce(ctx context.Context, source string, target string, reportSpeed func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	resp, err := d.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpStatusDownloadError{StatusCode: resp.StatusCode, Status: resp.Status, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	tmp := target + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	copyErr := copyWithSpeedAndIdleTimeout(ctx, file, resp.Body, videoDownloadIdleTimeout, reportSpeed)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, target)
}

func (d TwitterDownloader) sleep(ctx context.Context, delay time.Duration) error {
	if d.RetrySleeper != nil {
		return d.RetrySleeper(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d TwitterDownloader) downloadThumbnail(ctx context.Context, tweet *twitterscraper.TweetResult, video twitterscraper.Video, videoURL string, videoDir string) error {
	if strings.TrimSpace(video.Preview) == "" {
		return nil
	}
	name := TWMDMediaFileName(time.Unix(tweet.Timestamp, 0), videoURL, tweet.Text, ".jpg")
	return d.download(ctx, video.Preview, filepath.Join(videoDir, name), nil)
}

func copyWithSpeed(dst io.Writer, src io.Reader, reportSpeed func(int64)) error {
	return copyWithSpeedAndIdleTimeout(context.Background(), dst, io.NopCloser(src), 0, reportSpeed)
}

func copyWithSpeedAndIdleTimeout(ctx context.Context, dst io.Writer, src io.ReadCloser, idleTimeout time.Duration, reportSpeed func(int64)) error {
	buffer := make([]byte, 64*1024)
	windowStart := time.Now()
	var windowBytes int64
	for {
		n, readErr := readWithIdleTimeout(ctx, src, buffer, idleTimeout)
		if n > 0 {
			if _, err := dst.Write(buffer[:n]); err != nil {
				return err
			}
			if reportSpeed != nil {
				windowBytes += int64(n)
				elapsed := time.Since(windowStart)
				if elapsed >= 500*time.Millisecond {
					reportSpeed(int64(float64(windowBytes) / elapsed.Seconds()))
					windowStart = time.Now()
					windowBytes = 0
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				if reportSpeed != nil && windowBytes > 0 {
					elapsed := time.Since(windowStart)
					if elapsed > 0 {
						reportSpeed(int64(float64(windowBytes) / elapsed.Seconds()))
					}
				}
				return nil
			}
			return readErr
		}
	}
}

type readResult struct {
	n   int
	err error
}

func readWithIdleTimeout(ctx context.Context, src io.ReadCloser, buffer []byte, idleTimeout time.Duration) (int, error) {
	if idleTimeout <= 0 {
		return src.Read(buffer)
	}
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := src.Read(buffer)
		resultCh <- readResult{n: n, err: err}
	}()
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		_ = src.Close()
		return 0, ctx.Err()
	case <-timer.C:
		_ = src.Close()
		return 0, idleReadTimeoutError{Timeout: idleTimeout}
	case result := <-resultCh:
		return result.n, result.err
	}
}

func isRetryableDownloadError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr httpStatusDownloadError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var idleErr idleReadTimeoutError
	if errors.As(err, &idleErr) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{
		"client.timeout", "timeout", "connection reset", "connection refused", "broken pipe", "unexpected eof", "stream error", "temporary failure",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func retryDelayForDownloadError(err error, attempt int) time.Duration {
	var statusErr httpStatusDownloadError
	if errors.As(err, &statusErr) && statusErr.RetryAfter > 0 {
		if statusErr.RetryAfter > videoDownloadMaxRetryAfterWait {
			return videoDownloadMaxRetryAfterWait
		}
		return statusErr.RetryAfter
	}
	index := attempt - 1
	if index < 0 {
		index = 0
	}
	if index >= len(videoDownloadBackoffs) {
		index = len(videoDownloadBackoffs) - 1
	}
	delay := videoDownloadBackoffs[index]
	jitter := time.Duration(time.Now().UnixNano()%500) * time.Millisecond
	return delay + jitter
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		return time.Until(when)
	}
	return 0
}

func formatRetryDelay(delay time.Duration) string {
	seconds := int(delay.Round(time.Second).Seconds())
	if seconds <= 0 {
		return "稍后"
	}
	return strconv.Itoa(seconds) + " 秒"
}

func TWMDMediaFileName(tweetTime time.Time, mediaURL string, text string, ext string) string {
	if ext == "" {
		parsedExt := filepath.Ext(strings.Split(mediaURL, "?")[0])
		if parsedExt != "" {
			ext = parsedExt
		}
	}
	base := twmdURLBaseName(mediaURL)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	content := sanitizeTWMDText(text, 20)
	if content == "" {
		content = "没有推文"
	}
	return tweetTime.Format("2006-01-02") + "_" + base + "_" + content + ext
}

func twmdSidecarName(tweetTime time.Time, videoURL string, text string, ext string) string {
	return TWMDMediaFileName(tweetTime, videoURL, text, ext)
}

func twmdURLBaseName(raw string) string {
	segment := filepath.Base(strings.Split(raw, "?")[0])
	if strings.Contains(raw, "name=") {
		if parsed, err := url.Parse(raw); err == nil {
			if name := parsed.Query().Get("name"); name != "" {
				segment = name
			}
		}
	}
	return segment
}

func sanitizeTWMDText(value string, limit int) string {
	re := regexp.MustCompile(`[/\\:*?"<>|]`)
	value = re.ReplaceAllString(value, "")
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if limit > 0 && len(runes) > limit {
		runes = runes[:limit]
	}
	return strings.TrimSpace(string(runes))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func originalTWMDName(name string) string {
	parts := strings.SplitN(name, "_", 3)
	if len(parts) < 2 {
		return name
	}
	return parts[1] + filepath.Ext(name)
}

func writeTWMDJSON(tweet *twitterscraper.TweetResult, videoURL string, videoDir string) error {
	name := twmdSidecarName(time.Unix(tweet.Timestamp, 0), videoURL, tweet.Text, ".json")
	data, err := json.MarshalIndent(tweet, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(videoDir, name), data, 0o644)
}

func writeTWMDNFO(tweet *twitterscraper.TweetResult, videoURL string, videoDir string) error {
	name := twmdSidecarName(time.Unix(tweet.Timestamp, 0), videoURL, tweet.Text, ".nfo")
	content := "Title: " + tweet.Text + "\nSource: " + tweet.PermanentURL + "\nID: " + tweet.ID + "\n"
	return os.WriteFile(filepath.Join(videoDir, name), []byte(content), 0o644)
}

func writeTWMDASS(tweet *twitterscraper.TweetResult, videoURL string, videoDir string) error {
	name := twmdSidecarName(time.Unix(tweet.Timestamp, 0), videoURL, tweet.Text, ".ass")
	content := "[Script Info]\nTitle: Twitter Video Subtitle\n\n[Events]\nDialogue: 0,0:00:00.00,0:00:05.00,Default,,0,0,0,," + strings.ReplaceAll(tweet.Text, "\n", " ") + "\n"
	return os.WriteFile(filepath.Join(videoDir, name), []byte(content), 0o644)
}
