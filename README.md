# Omran Video Studio

أداة محلية خفيفة لشركة عمران التجارية لإنشاء سكربتات UGC مصرية وتجهيز فيديوهات رأسية.

## التشغيل

```bash
go test ./...
go run ./cmd/studio
```

افتح `http://localhost:8080`.

## تصدير فيديو رأسي

```bash
go run ./cmd/render -image assets/product.jpg -watermark assets/omran-watermark.png -seconds 20 -output output/video.mp4
```

الصوت اختياري عبر `-audio`. التصدير: MP4 H.264، مقاس 1080×1920، و30fps.

## المبادئ

- ريبو مستقل تمامًا عن متجر عمران.
- Local-first ومن دون API أو Secrets.
- لغة مصرية وهوية عمران الصحيحة.
- مراجعة بشرية قبل نشر أي ادعاء أو سعر.
- لا تُحفظ ملفات الفيديو الكبيرة داخل Git.
