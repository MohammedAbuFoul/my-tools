package main

import (
	"regexp"
	"strings"

	"github.com/atotto/clipboard"
)

var youtubeURLRegex = regexp.MustCompile(`(?i)^https?://(?:www\.|m\.|music\.)?(?:youtube\.com/(?:watch\?.*v=[\w-]+|shorts/[\w-]+|playlist\?.*list=[\w-]+|live/[\w-]+)|youtu\.be/[\w-]+)`)

func IsYouTubeURL(input string) bool {
	s := strings.TrimSpace(input)
	return youtubeURLRegex.MatchString(s)
}

func ReadClipboardYouTubeURL() (string, bool) {
	text, err := clipboard.ReadAll()
	if err != nil {
		return "", false
	}
	text = strings.TrimSpace(text)
	if IsYouTubeURL(text) {
		return text, true
	}
	return "", false
}

func ReadClipboardRaw() string {
	text, err := clipboard.ReadAll()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

