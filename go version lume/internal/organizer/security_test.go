package organizer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lume-go/internal/metadata"
)

// EXIF cihaz adi saldirgan kontrolunde ve klasor yoluna akiyor.
// Bu deger ile dizin traversal (hedef kok disina yazma) denenir.
func TestSecurity_DeviceNamePathTraversal(t *testing.T) {
	attacks := []string{
		`..\..\..\Windows\System32`,
		`../../../../etc/cron.d`,
		`....//....//evil`,
		`foo/../../bar`,
		`\?\C:\Windows`,
		`C:\Windows\System32`,
		`a\x00b`,
		strings.Repeat("../", 40) + "pwn",
		`.`,
		`..`,
		`con`, `PRN`, `NUL`, `COM1`, `LPT9`,
		"normal\x01\x1fname",
	}
	for _, a := range attacks {
		got := SanitizeFolderName(a)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("TRAVERSAL: SanitizeFolderName(%q) = %q ayirici iceriyor", a, got)
		}
		if strings.Contains(got, "..") {
			t.Errorf("TRAVERSAL: SanitizeFolderName(%q) = %q '..' iceriyor", a, got)
		}
		if got == "." || got == ".." || got == "" {
			t.Errorf("TRAVERSAL: SanitizeFolderName(%q) = %q tehlikeli sonuc", a, got)
		}
		// sonucun tek bir yol segmenti oldugunu dogrula
		if filepath.Base(got) != got {
			t.Errorf("TRAVERSAL: SanitizeFolderName(%q) = %q cok-segmentli", a, got)
		}
	}
}

// Saldirgan EXIF cihaz adiyla gercek arsivleme akisini surer ve
// yazilan dosyanin hedef kok DISINA cikip cikmadigini dosya sisteminde olcer.
func TestSecurity_ArchiveCannotEscapeTargetRoot(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "arsiv")
	outside := filepath.Join(base, "DISARIDA")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(base, "kaynak.jpg")
	if err := os.WriteFile(src, []byte("zararsiz icerik"), 0644); err != nil {
		t.Fatal(err)
	}

	evil := []string{
		`..\..\DISARIDA`,
		`../../DISARIDA`,
		strings.Repeat("../", 10) + "DISARIDA",
		`\?\` + outside,
	}
	for _, dev := range evil {
		info := metadata.FileInfo{
			Path:     src,
			Filename: "kaynak.jpg",
			Year:     "2024",
			Month:    "01",
			Device:   dev,
			Source:   "",
		}
		final, _, err := ArchiveFileWithOptions(context.Background(), info, target, false, false, NewState())
		if err != nil {
			continue // reddedildi = guvenli
		}
		abs, _ := filepath.Abs(final)
		absTarget, _ := filepath.Abs(target)
		if !strings.HasPrefix(abs, absTarget+string(filepath.Separator)) {
			t.Errorf("KACIS: cihaz=%q -> dosya hedef disina yazildi: %s", dev, abs)
		}
	}

	// hedef kokun disinda hicbir sey olusmamali
	if _, err := os.Stat(outside); err == nil {
		t.Errorf("KACIS: hedef disi dizin olustu: %s", outside)
	}
}

// Bozuk/asiri EXIF verisi panik veya kilitlenme yaratmamali (DoS).
func TestSecurity_MalformedExifDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	payloads := map[string][]byte{
		"bos.jpg":       {},
		"kirpik.jpg":    []byte("\xff\xd8\xff\xe1\x00\x10Exif\x00\x00"), // yarim EXIF basligi
		"cop.jpg":       []byte(strings.Repeat("\xff\xe1\xff\xff", 5000)),
		"buyuk_tag.jpg": append([]byte("\xff\xd8\xff\xe1\xff\xfeExif\x00\x00II*\x00\x08\x00\x00\x00"), []byte(strings.Repeat("A", 100000))...),
	}
	for name, data := range payloads {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0644); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("DoS: %s EXIF ayristirmasi panikledi: %v", name, r)
				}
			}()
			_, _, _ = metadata.ExtractExif(p) // panik yutulmali, hata donebilir
		}()
	}
}
