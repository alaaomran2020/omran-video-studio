package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alaaomran2020/omran-video-studio/internal/render"
)

type Scene struct {
	Image   string `json:"image"`
	Seconds int    `json:"seconds"`
	Caption string `json:"caption,omitempty"`
}

type Project struct {
	Name      string  `json:"name"`
	Template  string  `json:"template"`
	Audio     string  `json:"audio,omitempty"`
	Watermark string  `json:"watermark,omitempty"`
	Output    string  `json:"output"`
	Scenes    []Scene `json:"scenes"`
}

type Template struct{ ID, Name, SubtitleStyle string }

var templates = map[string]Template{
	"classic": {"classic", "عمران كلاسيك", "FontName=DejaVu Sans,FontSize=19,PrimaryColour=&H00FFFFFF,OutlineColour=&H00111111,BorderStyle=1,Outline=2,Alignment=2,MarginV=110"},
	"warm":    {"warm", "دافئ للعيلة", "FontName=DejaVu Sans,FontSize=20,PrimaryColour=&H0000E6FF,OutlineColour=&H00201A10,BorderStyle=1,Outline=2,Alignment=2,MarginV=110"},
	"catalog": {"catalog", "كتالوج سريع", "FontName=DejaVu Sans,FontSize=18,PrimaryColour=&H00FFFFFF,OutlineColour=&H00752B12,BorderStyle=3,Outline=1,Alignment=2,MarginV=100"},
}

func Templates() []Template {
	return []Template{templates["classic"], templates["warm"], templates["catalog"]}
}

func Validate(p Project) error {
	if len(p.Scenes) < 1 || len(p.Scenes) > 20 {
		return errors.New("عدد المشاهد يجب أن يكون من 1 إلى 20")
	}
	if _, ok := templates[p.Template]; !ok {
		return errors.New("القالب غير مدعوم")
	}
	clean := filepath.Clean(p.Output)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || !strings.HasPrefix(filepath.ToSlash(clean), "output/") || strings.ToLower(filepath.Ext(clean)) != ".mp4" {
		return errors.New("الإخراج يجب أن يكون MP4 داخل output/")
	}
	total := 0
	for i, s := range p.Scenes {
		if s.Seconds < 1 || s.Seconds > 60 {
			return fmt.Errorf("مدة المشهد %d غير صالحة", i+1)
		}
		total += s.Seconds
		if total > 300 {
			return errors.New("إجمالي المدة لا يتجاوز 5 دقائق")
		}
		if err := media(s.Image, ".jpg", ".jpeg", ".png", ".webp"); err != nil {
			return fmt.Errorf("المشهد %d: %w", i+1, err)
		}
	}
	if p.Audio != "" {
		if err := media(p.Audio, ".wav", ".mp3", ".m4a", ".aac"); err != nil {
			return err
		}
	}
	if p.Watermark != "" {
		if err := media(p.Watermark, ".png", ".webp"); err != nil {
			return err
		}
	}
	return nil
}

func media(path string, exts ...string) error {
	ext := strings.ToLower(filepath.Ext(path))
	for _, allowed := range exts {
		if ext == allowed {
			if _, err := os.Stat(path); err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("صيغة غير مدعومة: %s", ext)
}

func SRT(scenes []Scene) string {
	var b strings.Builder
	start := time.Duration(0)
	n := 1
	for _, scene := range scenes {
		end := start + time.Duration(scene.Seconds)*time.Second
		if strings.TrimSpace(scene.Caption) != "" {
			fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, srtTime(start), srtTime(end), cleanCaption(scene.Caption))
			n++
		}
		start = end
	}
	return b.String()
}

func srtTime(d time.Duration) string {
	h := d / time.Hour
	d %= time.Hour
	m := d / time.Minute
	d %= time.Minute
	s := d / time.Second
	ms := (d % time.Second) / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

var tags = regexp.MustCompile("<[^>]*>")

func cleanCaption(s string) string {
	s = tags.ReplaceAllString(strings.TrimSpace(s), "")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\n", " ")
}

func Run(ctx context.Context, p Project) error {
	if err := Validate(p); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.Output), 0755); err != nil {
		return err
	}
	work := filepath.Join("output", ".work-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.MkdirAll(work, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(work)

	var clips []string
	for i, scene := range p.Scenes {
		clip := filepath.Join(work, fmt.Sprintf("scene-%03d.mp4", i+1))
		if err := render.Run(ctx, render.Options{Image: scene.Image, Watermark: p.Watermark, Output: clip, Seconds: scene.Seconds}); err != nil {
			return fmt.Errorf("فشل المشهد %d: %w", i+1, err)
		}
		clips = append(clips, clip)
	}
	listPath := filepath.Join(work, "concat.txt")
	var list strings.Builder
	for _, clip := range clips {
		fmt.Fprintf(&list, "file '%s'\n", filepath.Base(clip))
	}
	if err := os.WriteFile(listPath, []byte(list.String()), 0600); err != nil {
		return err
	}
	base := filepath.Join(work, "base.mp4")
	if err := command(ctx, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", base); err != nil {
		return err
	}

	srt := SRT(p.Scenes)
	srtPath := strings.TrimSuffix(p.Output, ".mp4") + ".srt"
	if srt != "" {
		if err := os.WriteFile(srtPath, []byte(srt), 0644); err != nil {
			return err
		}
	}
	args := []string{"-y", "-i", base}
	if p.Audio != "" {
		args = append(args, "-i", p.Audio)
	}
	if srt != "" {
		args = append(args, "-vf", fmt.Sprintf("subtitles=%s:force_style='%s'", escapePath(srtPath), templates[p.Template].SubtitleStyle))
	}
	args = append(args, "-map", "0:v:0")
	if p.Audio != "" {
		args = append(args, "-map", "1:a:0?", "-c:a", "aac", "-shortest")
	} else {
		args = append(args, "-an")
	}
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", p.Output)
	return command(ctx, args...)
}

func escapePath(path string) string {
	return filepath.ToSlash(path)
}

func command(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func LoadProjects(path string) ([]Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, errors.New("ملف الدفعة فارغ")
	}
	return projects, nil
}
