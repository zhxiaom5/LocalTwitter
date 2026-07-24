package server

import (
	"context"
	"net/http"
	"regexp"
	"strings"
)

type BulkImportItem struct {
	Input             string `json:"input"`
	ProfileURL        string `json:"profile_url,omitempty"`
	Username          string `json:"username,omitempty"`
	Status            string `json:"status"`
	Existing          bool   `json:"existing"`
	ExistingCreatorID int64  `json:"existing_creator_id,omitempty"`
	Error             string `json:"error,omitempty"`
}

var twitterBulkURLPattern = regexp.MustCompile(`(?i)(?:https?://)?(?:www\.)?(?:x\.com|twitter\.com)/[^\s"'<>()，。]+`)
var twitterMentionPattern = regexp.MustCompile(`(?i)(?:^|[\s，,。:：])@([A-Za-z0-9_]{1,15})\b`)

func PreviewBulkImportCreators(ctx context.Context, store *Store, directoryID int64, text string, client *http.Client) []BulkImportItem {
	seen := map[string]bool{}
	items := []BulkImportItem{}
	for _, line := range strings.Split(text, "\n") {
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		candidates := extractTwitterCandidates(input)
		if len(candidates) == 0 {
			items = append(items, BulkImportItem{Input: input, Status: "error", Error: "no twitter/x user found"})
			continue
		}
		for _, raw := range candidates {
			profileURL, username, err := normalizeTwitterProfileURL(ctx, raw, client)
			if err != nil {
				items = append(items, BulkImportItem{Input: input, Status: "error", Error: err.Error()})
				continue
			}
			if seen[profileURL] {
				continue
			}
			seen[profileURL] = true
			item := BulkImportItem{Input: input, ProfileURL: profileURL, Username: username, Status: "ready"}
			if store != nil {
				if creator, ok, err := store.CreatorByOnlineIdentity(directoryID, username, profileURL); err == nil && ok {
					item.Existing = true
					item.ExistingCreatorID = creator.ID
				}
			}
			items = append(items, item)
		}
	}
	return items
}

func extractTwitterCandidates(input string) []string {
	candidates := make([]string, 0)
	for _, raw := range twitterBulkURLPattern.FindAllString(input, -1) {
		candidates = append(candidates, trimURLNoise(raw))
	}
	if len(candidates) > 0 {
		return candidates
	}
	if match := twitterMentionPattern.FindStringSubmatch(input); len(match) == 2 {
		return []string{match[1]}
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(input), "@")
	if twitterUsernamePattern.MatchString(trimmed) {
		return []string{trimmed}
	}
	return candidates
}

func trimURLNoise(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), ".,，。!?！？)）]")
}
