package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/alaaomran2020/omran-video-studio/internal/render"
)

func main() {
	image := flag.String("image", "", "product image")
	audio := flag.String("audio", "", "optional audio")
	watermark := flag.String("watermark", "", "optional transparent PNG")
	output := flag.String("output", "output/video.mp4", "output MP4")
	seconds := flag.Int("seconds", 20, "duration, 1-60")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := render.Run(ctx, render.Options{Image: *image, Audio: *audio, Watermark: *watermark, Output: *output, Seconds: *seconds}); err != nil {
		log.Fatal(err)
	}
}
