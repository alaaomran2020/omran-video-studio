package render

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArgs(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "product.jpg")
	if err := os.WriteFile(image, []byte("demo"), 0600); err != nil {
		t.Fatal(err)
	}
	args, err := Args(Options{Image: image, Output: "output/test.mp4", Seconds: 15})
	if err != nil || len(args) == 0 {
		t.Fatalf("تعذر بناء أمر آمن: %v", err)
	}
}

func TestRejectUnsafeOutput(t *testing.T) {
	if err := outputPath("../video.mp4"); err == nil {
		t.Fatal("تم قبول مسار غير آمن")
	}
}
