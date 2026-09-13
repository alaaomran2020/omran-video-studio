package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alaaomran2020/omran-video-studio/internal/render"
	"github.com/alaaomran2020/omran-video-studio/internal/script"
)

var renderSlot = make(chan struct{}, 1)

func main() {
	port := flag.String("port", "8080", "HTTP port")
	web := flag.String("web", "web", "web directory")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
	mux.HandleFunc("/api/script", handleScript)
	mux.HandleFunc("/api/render", handleRender)
	mux.Handle("/output/", http.StripPrefix("/output/", http.FileServer(http.Dir("output"))))
	mux.Handle("/", http.FileServer(http.Dir(*web)))
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(out)
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
	os.MkdirAll("tmp", 0755)
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": "/output/" + name})
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
	for _, a := range allowed {
		ok = ok || ext == a
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

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; media-src 'self' blob:")
		next.ServeHTTP(w, r)
	})
}
