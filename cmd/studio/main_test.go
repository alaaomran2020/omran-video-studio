package main

import (
	"testing"
	"time"
)

func TestJobStoreLifecycle(t *testing.T) {
	store := jobStore{items: make(map[string]job)}
	now := time.Now()
	store.put(job{ID: "one", Name: "تجربة", Status: "queued", CreatedAt: now, UpdatedAt: now})
	store.update("one", "done", "/output/one.mp4", "/output/one.srt", "")
	items := store.list()
	if len(items) != 1 || items[0].Status != "done" || items[0].OutputURL == "" || items[0].SRTURL == "" {
		t.Fatalf("unexpected job: %#v", items)
	}
}
