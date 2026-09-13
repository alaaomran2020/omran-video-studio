package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/alaaomran2020/omran-video-studio/internal/pipeline"
)

type result struct {
	Name   string `json:"name"`
	Output string `json:"output"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func main() {
	input := flag.String("input", "batch.json", "ملف JSON يحتوي على قائمة مشاريع")
	timeout := flag.Duration("timeout", 20*time.Minute, "مهلة الدفعة")
	flag.Parse()
	projects, err := pipeline.LoadProjects(*input)
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	results := make([]result, 0, len(projects))
	failed := false
	for _, p := range projects {
		r := result{Name: p.Name, Output: p.Output, Status: "done"}
		if err := pipeline.Run(ctx, p); err != nil {
			r.Status, r.Error, failed = "failed", err.Error(), true
		}
		results = append(results, r)
	}
	report, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(report))
	if failed {
		os.Exit(1)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
