package script

import (
	"strings"
	"testing"
)

func TestGenerateAndNormalize(t *testing.T) {
	out, err := Generate(Input{Name: "سيارات أطفال من أمران", Benefit: "لعبة حركة", Duration: 20})
	if err != nil {
		t.Fatal(err)
	}
	all := out.Caption + strings.Join(out.Hooks, " ")
	if strings.Contains(all, "أمران") || strings.Contains(all, "سيارات") {
		t.Fatal("لم يتم تطبيق لغة وهوية عمران")
	}
	if len(out.Scenes) != 4 || out.RecommendedHook == "" {
		t.Fatal("مخرجات غير مكتملة")
	}
}

func TestRequiredFields(t *testing.T) {
	if _, err := Generate(Input{}); err == nil {
		t.Fatal("كان يجب رفض البيانات الناقصة")
	}
}
