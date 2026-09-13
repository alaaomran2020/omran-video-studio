package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alaaomran2020/omran-video-studio/internal/pipeline"
)

func TestJobStorePersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jobs.json")
	store := newJobStore(path)
	now := time.Now()
	record := &jobRecord{Job: job{ID: "one", Name: "تجربة", Status: "queued", CreatedAt: now, UpdatedAt: now}}
	if err := store.put(record); err != nil {
		t.Fatal(err)
	}
	store.update("one", "done", "/output/one.mp4", "/output/one.srt", "")
	reloaded := newJobStore(path)
	items := reloaded.list()
	if len(items) != 1 || items[0].Status != "done" || items[0].OutputURL == "" || items[0].SRTURL == "" {
		t.Fatalf("unexpected persisted job: %#v", items)
	}
}

func TestRetryKeepsOriginalInputs(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "scene.jpg")
	if err := os.WriteFile(image, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	store := newJobStore(filepath.Join(dir, "jobs.json"))
	now := time.Now()
	record := &jobRecord{
		Job:       job{ID: "retry", Name: "إعادة", Status: "failed", CreatedAt: now, UpdatedAt: now},
		Project:   pipeline.Project{Template: "classic", Output: "output/retry.mp4", Scenes: []pipeline.Scene{{Image: image, Seconds: 3}}},
		UploadDir: dir,
	}
	if err := store.put(record); err != nil {
		t.Fatal(err)
	}
	project, uploadDir, err := store.retry("retry")
	if err != nil {
		t.Fatal(err)
	}
	if uploadDir != dir || project.Scenes[0].Image != image || store.list()[0].Status != "queued" {
		t.Fatal("retry did not preserve job inputs")
	}
}

func TestCancelSignalsRunningJob(t *testing.T) {
	dir := t.TempDir()
	store := newJobStore(filepath.Join(dir, "jobs.json"))
	now := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	record := &jobRecord{Job: job{ID: "cancel", Name: "إلغاء", Status: "queued", CreatedAt: now, UpdatedAt: now}}
	if err := store.put(record); err != nil {
		t.Fatal(err)
	}
	store.setCancel("cancel", cancel)
	if err := store.cancelJob("cancel"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("cancel function was not called")
	}
	if store.list()[0].Status != "cancelling" {
		t.Fatal("job was not marked as cancelling")
	}
}

func TestRestartResumesPendingButNotCancelled(t *testing.T) {
	dir := t.TempDir()
	store := newJobStore(filepath.Join(dir, "jobs.json"))
	now := time.Now()
	if err := store.put(&jobRecord{Job: job{ID: "queued", Status: "queued", CreatedAt: now, UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	if err := store.put(&jobRecord{Job: job{ID: "cancelled", Status: "cancelling", CreatedAt: now.Add(time.Second), UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	pending := store.resumable()
	if len(pending) != 1 || pending[0].Job.ID != "queued" {
		t.Fatalf("unexpected resumed jobs: %#v", pending)
	}
	items := store.list()
	for _, item := range items {
		if item.ID == "cancelled" && item.Status != "cancelled" {
			t.Fatalf("cancelled job resumed: %#v", item)
		}
	}
}
