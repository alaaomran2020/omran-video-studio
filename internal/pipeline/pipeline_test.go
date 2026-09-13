package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSRTUsesSceneTimeline(t *testing.T) {
	got := SRT([]Scene{{Seconds: 3, Caption: "أول لقطة"}, {Seconds: 4}, {Seconds: 2, Caption: "آخر لقطة"}})
	if !strings.Contains(got, "00:00:00,000 --> 00:00:03,000") || !strings.Contains(got, "00:00:07,000 --> 00:00:09,000") {
		t.Fatalf("unexpected SRT: %s", got)
	}
}

func TestValidateRejectsUnsafeOutput(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "one.jpg")
	if err := os.WriteFile(image, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	err := Validate(Project{Template: "classic", Output: "../bad.mp4", Scenes: []Scene{{Image: image, Seconds: 3}}})
	if err == nil {
		t.Fatal("expected unsafe output error")
	}
}

func TestTemplatesAreStable(t *testing.T) {
	if len(Templates()) != 3 {
		t.Fatalf("got %d templates", len(Templates()))
	}
}
