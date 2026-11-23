package main

import (
	"testing"
)

func TestIsYouTubeURL(t *testing.T) {
	valid := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://youtube.com/shorts/abc123XYZ_-",
		"https://music.youtube.com/watch?v=dQw4w9WgXcQ&list=RDAMVM",
		"https://www.youtube.com/playlist?list=PL1234567890",
	}
	for _, u := range valid {
		if !IsYouTubeURL(u) {
			t.Errorf("expected valid YouTube URL: %s", u)
		}
	}

	invalid := []string{
		"",
		"hello world",
		"https://google.com",
		"https://vimeo.com/12345",
	}
	for _, u := range invalid {
		if IsYouTubeURL(u) {
			t.Errorf("expected invalid YouTube URL: %s", u)
		}
	}
}

func TestProgressRegex(t *testing.T) {
	line := "[download]  45.2% of   52.30MiB at    4.12MiB/s ETA 00:07"
	m := progressRegex.FindStringSubmatch(line)
	if len(m) < 5 {
		t.Fatalf("expected match, got %v", m)
	}
	if m[1] != "45.2" || m[2] != "52.30MiB" || m[3] != "4.12MiB/s" || m[4] != "00:07" {
		t.Errorf("unexpected groups: %v", m[1:])
	}
}

