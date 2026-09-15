package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Options struct {
	Image, Audio, Watermark, Output string
	Seconds                         int
}

func media(path string, allowed ...string) error {
	if path == "" {
		return errors.New("مسار الملف مطلوب")
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, a := range allowed {
		if ext == a {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("تعذر قراءة %s: %w", path, err)
			}
			return nil
		}
	}
	return fmt.Errorf("صيغة غير مدعومة: %s", ext)
}

func outputPath(path string) error {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") ||
		!strings.HasPrefix(filepath.ToSlash(clean), "output/") ||
		strings.ToLower(filepath.Ext(clean)) != ".mp4" {
		return errors.New("الإخراج يجب أن يكون ملف MP4 داخل output/")
	}
	return os.MkdirAll(filepath.Dir(clean), 0755)
}

func Args(o Options) ([]string, error) {
	if o.Seconds < 1 || o.Seconds > 60 {
		return nil, errors.New("المدة يجب أن تكون من 1 إلى 60 ثانية")
	}
	if err := media(o.Image, ".jpg", ".jpeg", ".png", ".webp"); err != nil {
		return nil, err
	}
	if o.Audio != "" {
		if err := media(o.Audio, ".wav", ".mp3", ".m4a", ".aac"); err != nil {
			return nil, err
		}
	}
	if o.Watermark != "" {
		if err := media(o.Watermark, ".png", ".webp"); err != nil {
			return nil, err
		}
	}
	if err := outputPath(o.Output); err != nil {
		return nil, err
	}

	args := []string{"-y", "-loop", "1", "-i", o.Image}
	if o.Audio != "" {
		args = append(args, "-i", o.Audio)
	}
	if o.Watermark != "" {
		args = append(args, "-i", o.Watermark)
		wm := 1
		if o.Audio != "" {
			wm = 2
		}
		filter := fmt.Sprintf("[0:v]scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2,fps=30[base];[%d:v]scale=180:-1,format=rgba,colorchannelmixer=aa=0.65[wm];[base][wm]overlay=W-w-40:40[v]", wm)
		args = append(args, "-filter_complex", filter, "-map", "[v]")
	} else {
		args = append(args, "-vf", "scale=1080:1920:force_original_aspect_ratio=decrease,pad=1080:1920:(ow-iw)/2:(oh-ih)/2,fps=30")
	}
	if o.Audio != "" {
		args = append(args, "-map", "1:a:0?", "-c:a", "aac", "-shortest")
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-t", strconv.Itoa(o.Seconds), "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", o.Output)
	return args, nil
}

func Run(ctx context.Context, o Options) error {
	args, err := Args(o)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
