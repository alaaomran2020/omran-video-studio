package script

import (
	"errors"
	"fmt"
	"strings"
)

type Input struct {
	Name     string `json:"name"`
	Benefit  string `json:"benefit"`
	Category string `json:"category,omitempty"`
	Audience string `json:"audience,omitempty"`
	CTA      string `json:"cta,omitempty"`
	Duration int    `json:"duration_seconds"`
}

type Scene struct {
	Seconds   string `json:"seconds"`
	Shot      string `json:"shot"`
	Voiceover string `json:"voiceover"`
	OnScreen  string `json:"on_screen"`
}

type Output struct {
	RecommendedHook string   `json:"recommended_hook"`
	Hooks           []string `json:"hooks"`
	Scenes          []Scene  `json:"scenes"`
	Caption         string   `json:"caption"`
	Hashtags        []string `json:"hashtags"`
}

func clean(s string) string {
	r := strings.NewReplacer("أمران", "عمران", "سيارات", "عربيات", "دمى", "عرايس")
	return strings.TrimSpace(r.Replace(s))
}

func Generate(in Input) (Output, error) {
	in.Name, in.Benefit = clean(in.Name), clean(in.Benefit)
	if in.Name == "" || in.Benefit == "" {
		return Output{}, errors.New("اسم المنتج والفائدة الأساسية مطلوبان")
	}
	if in.Duration < 15 || in.Duration > 30 {
		in.Duration = 20
	}
	if clean(in.CTA) == "" {
		in.CTA = "للاستفسار عن التفاصيل والكميات، كلمنا على واتساب."
	}
	hooks := []string{
		fmt.Sprintf("لو بتدور على %s… بص على دي.", in.Benefit),
		fmt.Sprintf("استنى، شوف %s بتعمل إيه.", in.Name),
		fmt.Sprintf("دي أسرع طريقة تشوف بيها %s بوضوح.", in.Name),
	}
	end := in.Duration - 5
	return Output{
		RecommendedHook: hooks[1],
		Hooks:           hooks,
		Scenes: []Scene{
			{"0–3", "لقطة قريبة للمنتج أثناء الحركة أو فتح العلبة", hooks[1], "شوفها على الطبيعة"},
			{"3–10", "إظهار الاحتياج الحقيقي بدون تمثيل زائد", "كنت عايز حاجة واضحة ومناسبة من غير ما أحتار.", "اختيار واضح"},
			{fmt.Sprintf("10–%d", end), "تجربة فعلية مع الحجم والخامة والمحتويات", fmt.Sprintf("دي %s، وأهم حاجة فيها: %s.", in.Name, in.Benefit), in.Benefit},
			{fmt.Sprintf("%d–%d", end, in.Duration), "لقطة نهائية نظيفة مع علامة Omran Toys", clean(in.CTA), "عمران تويز — ثقة في الاختيار"},
		},
		Caption:  fmt.Sprintf("%s بشكل واضح ومن غير مبالغة. %s", in.Benefit, clean(in.CTA)),
		Hashtags: []string{"#لعب_أطفال", "#عمران_تويز", "#طنطا", "#هدايا_أطفال", "#ألعاب"},
	}, nil
}
