package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alaaomran2020/omran-video-studio/internal/pipeline"
	"github.com/alaaomran2020/omran-video-studio/internal/render"
	"github.com/alaaomran2020/omran-video-studio/internal/script"
)

var renderSlot = make(chan struct{}, 1)

type job struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	OutputURL string    `json:"output_url,omitempty"`
	SRTURL    string    `json:"srt_url,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type jobRecord struct {
	Job       job              `json:"job"`
	Project   pipeline.Project `json:"project"`
	UploadDir string           `json:"upload_dir"`
	cancel    context.CancelFunc
}

type jobStore struct {
	mu    sync.RWMutex
	path  string
	items map[string]*jobRecord
}

func newJobStore(path string) *jobStore {
	s := &jobStore{path: path, items: make(map[string]*jobRecord)}
	data, err := os.ReadFile(path)
	if err == nil {
		var records []*jobRecord
		if json.Unmarshal(data, &records) == nil {
			for _, record := range records {
				if record != nil && record.Job.ID != "" {
					s.items[record.Job.ID] = record
				}
			}
		}
	}
	return s
}

var jobs = newJobStore(filepath.Join("data", "jobs.json"))

func (s *jobStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	records := make([]*jobRecord, 0, len(s.items))
	for _, record := range s.items {
		copyRecord := *record
		copyRecord.cancel = nil
		records = append(records, &copyRecord)
	}
	sort.Slice(records, func(i, k int) bool { return records[i].Job.CreatedAt.Before(records[k].Job.CreatedAt) })
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *jobStore) put(record *jobRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[record.Job.ID] = record
	return s.persistLocked()
}

func (s *jobStore) update(id, status, outputURL, srtURL, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.items[id]
	if !ok {
		return
	}
	record.Job.Status, record.Job.OutputURL, record.Job.SRTURL = status, outputURL, srtURL
	record.Job.Error, record.Job.UpdatedAt = message, time.Now()
	if err := s.persistLocked(); err != nil {
		log.Printf("persist job %s: %v", id, err)
	}
}

func (s *jobStore) list() []job {
	s.mu.RLock()
	out := make([]job, 0, len(s.items))
	for _, record := range s.items {
		out = append(out, record.Job)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.After(out[k].CreatedAt) })
	return out
}

func (s *jobStore) setCancel(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	if record, ok := s.items[id]; ok {
		record.cancel = cancel
	}
	s.mu.Unlock()
}

func (s *jobStore) clearCancel(id string) {
	s.mu.Lock()
	if record, ok := s.items[id]; ok {
		record.cancel = nil
	}
	s.mu.Unlock()
}

func (s *jobStore) cancelJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.items[id]
	if !ok {
		return os.ErrNotExist
	}
	if record.Job.Status != "queued" && record.Job.Status != "running" {
		return errors.New("لا يمكن إلغاء هذه المهمة")
	}
	record.Job.Status, record.Job.UpdatedAt = "cancelling", time.Now()
	cancel := record.cancel
	if err := s.persistLocked(); err != nil {
		return err
	}
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *jobStore) retry(id string) (pipeline.Project, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.items[id]
	if !ok {
		return pipeline.Project{}, "", os.ErrNotExist
	}
	if record.Job.Status != "failed" && record.Job.Status != "cancelled" {
		return pipeline.Project{}, "", errors.New("إعادة المحاولة متاحة للمهام الفاشلة أو الملغاة فقط")
	}
	for _, scene := range record.Project.Scenes {
		if _, err := os.Stat(scene.Image); err != nil {
			return pipeline.Project{}, "", errors.New("ملفات المهمة الأصلية غير متاحة")
		}
	}
	record.Job.Status, record.Job.Error, record.Job.OutputURL, record.Job.SRTURL = "queued", "", "", ""
	record.Job.UpdatedAt = time.Now()
	if err := s.persistLocked(); err != nil {
		return pipeline.Project{}, "", err
	}
	return record.Project, record.UploadDir, nil
}

func (s *jobStore) resumable() []*jobRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*jobRecord, 0)
	for _, record := range s.items {
		if record.Job.Status == "cancelling" {
			record.Job.Status, record.Job.Error, record.Job.UpdatedAt = "cancelled", "تم إلغاء المهمة", time.Now()
			continue
		}
		if record.Job.Status == "queued" || record.Job.Status == "running" {
			record.Job.Status, record.Job.Error, record.Job.UpdatedAt = "queued", "", time.Now()
			copyRecord := *record
			out = append(out, &copyRecord)
		}
	}
	if len(out) > 0 {
		if err := s.persistLocked(); err != nil {
			log.Printf("persist resumed jobs: %v", err)
		}
	}
	return out
}

func main() {
	port := flag.String("port", "8080", "HTTP port")
	web := flag.String("web", "web", "web directory")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
	mux.HandleFunc("/api/script", handleScript)
	mux.HandleFunc("/api/render", handleRender)
	mux.HandleFunc("/api/templates", handleTemplates)
	mux.HandleFunc("/api/batch", handleBatch)
	mux.HandleFunc("/api/jobs", handleJobs)
	mux.HandleFunc("/api/jobs/", handleJobAction)
	mux.Handle("/output/", http.StripPrefix("/output/", http.FileServer(http.Dir("output"))))
	mux.Handle("/", http.FileServer(http.Dir(*web)))
	for _, record := range jobs.resumable() {
		go runJob(record.Job.ID, record.Project)
	}
	log.Printf("Omran Video Studio: http://localhost:%s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, headers(mux)))
}

func handleScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var in script.Input
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "بيانات غير صالحة", 400)
		return
	}
	out, err := script.Generate(in)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	select {
	case renderSlot <- struct{}{}:
		defer func() { <-renderSlot }()
	default:
		http.Error(w, "يوجد فيديو قيد التنفيذ", 429)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 80<<20)
	if r.ParseMultipartForm(80<<20) != nil {
		http.Error(w, "الملفات غير صالحة أو أكبر من 80MB", 400)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	dir, err := os.MkdirTemp("tmp", "render-")
	if err != nil {
		http.Error(w, "تعذر تجهيز الملفات", 500)
		return
	}
	defer os.RemoveAll(dir)
	image, err := saveUpload(r, "image", dir, true, ".jpg", ".jpeg", ".png", ".webp")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	audio, err := saveUpload(r, "audio", dir, false, ".wav", ".mp3", ".m4a", ".aac")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	watermark, err := saveUpload(r, "watermark", dir, false, ".png", ".webp")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	seconds, _ := strconv.Atoi(r.FormValue("seconds"))
	name := fmt.Sprintf("omran-%d.mp4", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := render.Run(ctx, render.Options{Image: image, Audio: audio, Watermark: watermark, Output: filepath.Join("output", name), Seconds: seconds}); err != nil {
		log.Printf("render failed: %v", err)
		http.Error(w, "تعذر تصدير الفيديو", 500)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": "/output/" + name})
}

func handleTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	writeJSON(w, http.StatusOK, pipeline.Templates())
}

type batchInput struct {
	Name     string `json:"name"`
	Template string `json:"template"`
	Scenes   []struct {
		Seconds int    `json:"seconds"`
		Caption string `json:"caption"`
	} `json:"scenes"`
}

func handleBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 250<<20)
	if err := r.ParseMultipartForm(250 << 20); err != nil {
		http.Error(w, "الدفعة غير صالحة أو أكبر من 250MB", 400)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	var in batchInput
	if err := json.Unmarshal([]byte(r.FormValue("manifest")), &in); err != nil {
		http.Error(w, "بيانات المشاهد غير صالحة", 400)
		return
	}
	if len(in.Scenes) < 1 || len(in.Scenes) > 20 {
		http.Error(w, "عدد المشاهد من 1 إلى 20", 400)
		return
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	dir := filepath.Join("data", "jobs", id, "inputs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		http.Error(w, "تعذر تجهيز الدفعة", 500)
		return
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(filepath.Dir(dir))
		}
	}()
	audio, err := saveUpload(r, "audio", dir, false, ".wav", ".mp3", ".m4a", ".aac")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	watermark, err := saveUpload(r, "watermark", dir, false, ".png", ".webp")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	scenes := make([]pipeline.Scene, 0, len(in.Scenes))
	for i, item := range in.Scenes {
		image, err := saveUpload(r, fmt.Sprintf("scene_%d", i), dir, true, ".jpg", ".jpeg", ".png", ".webp")
		if err != nil {
			http.Error(w, fmt.Sprintf("المشهد %d: %v", i+1, err), 400)
			return
		}
		scenes = append(scenes, pipeline.Scene{Image: image, Seconds: item.Seconds, Caption: item.Caption})
	}
	output := filepath.Join("output", "batch-"+id+".mp4")
	project := pipeline.Project{Name: strings.TrimSpace(in.Name), Template: in.Template, Audio: audio, Watermark: watermark, Output: output, Scenes: scenes}
	if project.Name == "" {
		project.Name = "فيديو عمران"
	}
	if err := pipeline.Validate(project); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	now := time.Now()
	record := &jobRecord{Job: job{ID: id, Name: project.Name, Status: "queued", CreatedAt: now, UpdatedAt: now}, Project: project, UploadDir: filepath.Dir(dir)}
	if err := jobs.put(record); err != nil {
		http.Error(w, "تعذر حفظ المهمة", 500)
		return
	}
	cleanup = false
	go runJob(id, project)
	writeJSON(w, http.StatusAccepted, record.Job)
}

func runJob(id string, project pipeline.Project) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	jobs.setCancel(id, cancel)
	defer cancel()
	defer jobs.clearCancel(id)
	select {
	case renderSlot <- struct{}{}:
		defer func() { <-renderSlot }()
	case <-ctx.Done():
		jobs.update(id, "cancelled", "", "", "تم إلغاء المهمة")
		return
	}
	if ctx.Err() != nil {
		jobs.update(id, "cancelled", "", "", "تم إلغاء المهمة")
		return
	}
	jobs.update(id, "running", "", "", "")
	if err := pipeline.Run(ctx, project); err != nil {
		if ctx.Err() != nil {
			jobs.update(id, "cancelled", "", "", "تم إلغاء المهمة")
			return
		}
		log.Printf("batch %s failed: %v", id, err)
		jobs.update(id, "failed", "", "", "تعذر تصدير الفيديو")
		return
	}
	name := filepath.Base(project.Output)
	srtPath := strings.TrimSuffix(project.Output, ".mp4") + ".srt"
	srtURL := ""
	if _, err := os.Stat(srtPath); err == nil {
		srtURL = "/output/" + filepath.Base(srtPath)
	}
	jobs.update(id, "done", "/output/"+name, srtURL, "")
}

func handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	writeJSON(w, http.StatusOK, jobs.list())
}

func handleJobAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/jobs/"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" {
		http.Error(w, "مسار غير صالح", 404)
		return
	}
	id, action := parts[0], parts[1]
	switch action {
	case "cancel":
		if err := jobs.cancelJob(id); err != nil {
			jobActionError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
	case "retry":
		project, _, err := jobs.retry(id)
		if err != nil {
			jobActionError(w, err)
			return
		}
		go runJob(id, project)
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
	default:
		http.Error(w, "إجراء غير معروف", 404)
	}
}

func jobActionError(w http.ResponseWriter, err error) {
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "المهمة غير موجودة", 404)
		return
	}
	http.Error(w, err.Error(), http.StatusConflict)
}

func saveUpload(r *http.Request, field, dir string, required bool, allowed ...string) (string, error) {
	f, h, err := r.FormFile(field)
	if err != nil {
		if !required && err == http.ErrMissingFile {
			return "", nil
		}
		return "", fmt.Errorf("%s مطلوب", field)
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(h.Filename))
	ok := false
	for _, allowedExt := range allowed {
		ok = ok || ext == allowedExt
	}
	if !ok {
		return "", fmt.Errorf("صيغة %s غير مدعومة", field)
	}
	path := filepath.Join(dir, field+ext)
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	_, err = io.Copy(dst, f)
	return path, err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; media-src 'self' blob:")
		next.ServeHTTP(w, r)
	})
}
