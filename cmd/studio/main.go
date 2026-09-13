package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/alaaomran2020/omran-video-studio/internal/script"
)

func main() {
	port := flag.String("port", "8080", "HTTP port")
	web := flag.String("web", "web", "web assets directory")
	flag.Parse()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/api/script", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var in script.Input
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
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
	})
	mux.Handle("/", http.FileServer(http.Dir(*web)))
	log.Printf("Omran Video Studio: http://localhost:%s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, securityHeaders(mux)))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")
		next.ServeHTTP(w, r)
	})
}
