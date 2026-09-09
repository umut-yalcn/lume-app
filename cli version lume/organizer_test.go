package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("dizin oluşturulamadı: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("dosya yazılamadı: %v", err)
	}
	return path
}

func newTestOrganizer(t *testing.T, dst string, dryRun, rename bool) *Organizer {
	t.Helper()
	return &Organizer{
		absDst:       dst,
		totalFiles:   0,
		createdDirs:  make(map[string]bool),
		sigChan:      make(chan os.Signal, 1),
		dryRun:       dryRun,
		rename:       rename,
		plannedPaths: make(map[string]bool),
		plannedMeta:  make(map[string]plannedFile),
		ctx:          t.Context(),
	}
}

func entryFor(t *testing.T, path string) fileEntry {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return fileEntry{path: path, size: info.Size(), modTime: info.ModTime(), mode: info.Mode()}
}

func TestFileHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, filepath.Join(dir, "a.jpg"), "aynı içerik")
	b := writeFile(t, filepath.Join(dir, "b.jpg"), "aynı içerik")

	ha, err := fileHash(a)
	if err != nil {
		t.Fatalf("fileHash(a): %v", err)
	}
	hb, err := fileHash(b)
	if err != nil {
		t.Fatalf("fileHash(b): %v", err)
	}
	if ha != hb {
		t.Errorf("aynı içerikli dosyalar farklı hash üretti: %s != %s", ha, hb)
	}
	if len(ha) != 64 {
		t.Errorf("SHA-256 hex uzunluğu 64 olmalı, %d bulundu", len(ha))
	}
}

func TestCopyAndHashFile_ContentAndHash(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, filepath.Join(dir, "src.jpg"), "içerik doğrulaması")
	dst := filepath.Join(dir, "out", "dst.jpg")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatal(err)
	}

	hash, err := copyAndHashFile(t.Context(), src, dst, 0644)
	if err != nil {
		t.Fatalf("copyAndHashFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("hedef okunamadı: %v", err)
	}
	if string(got) != "içerik doğrulaması" {
		t.Errorf("hedef içeriği bozuldu: %q", string(got))
	}

	dstHash, err := fileHash(dst)
	if err != nil {
		t.Fatal(err)
	}
	if hash != dstHash {
		t.Errorf("kopyalama sırasında dönen hash hedefinkiyle uyuşmuyor")
	}
}

func TestProcess_DuplicateIsSkipped(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	a := writeFile(t, filepath.Join(src, "foto.jpg"), "aynı")
	b := writeFile(t, filepath.Join(src, "alt", "foto.jpg"), "aynı")

	org := newTestOrganizer(t, dst, false, false)
	org.totalFiles = 2
	if err := org.Process(entryFor(t, a)); err != nil {
		t.Fatalf("ilk dosya: %v", err)
	}
	if err := org.Process(entryFor(t, b)); err != nil {
		t.Fatalf("ikinci dosya: %v", err)
	}

	if org.success != 1 {
		t.Errorf("success = %d; yalnız bir kopya arşivlenmeliydi", org.success)
	}
	if org.duplicates != 1 {
		t.Errorf("duplicates = %d; ikinci dosya kopya sayılmalıydı", org.duplicates)
	}
	if org.errors != 0 {
		t.Errorf("errors = %d; hata beklenmiyordu", org.errors)
	}
}

func TestProcess_DifferentContentGetsSuffix(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	a := writeFile(t, filepath.Join(src, "foto.jpg"), "birinci")
	b := writeFile(t, filepath.Join(src, "alt", "foto.jpg"), "ikinci içerik")

	stamp := time.Date(2024, 1, 15, 12, 30, 45, 0, time.Local)
	for _, p := range []string{a, b} {
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}

	org := newTestOrganizer(t, dst, false, false)
	org.totalFiles = 2
	if err := org.Process(entryFor(t, a)); err != nil {
		t.Fatal(err)
	}
	if err := org.Process(entryFor(t, b)); err != nil {
		t.Fatal(err)
	}

	if org.success != 2 {
		t.Fatalf("success = %d; farklı içerikli iki dosya da arşivlenmeliydi", org.success)
	}
	base := filepath.Join(dst, "2024", "01", "15")
	for _, name := range []string{"foto.jpg", "foto_1.jpg"} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Errorf("%s bulunamadı: %v", name, err)
		}
	}
}

func TestProcess_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	a := writeFile(t, filepath.Join(src, "foto.jpg"), "simülasyon")

	org := newTestOrganizer(t, dst, true, false)
	org.totalFiles = 1
	if err := org.Process(entryFor(t, a)); err != nil {
		t.Fatal(err)
	}

	if org.success != 1 {
		t.Errorf("success = %d; simülasyon başarı saymalıydı", org.success)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("dry-run modunda hedef klasör oluşturulmamalıydı")
	}
}

func TestProcess_RenameUsesTimestamp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	a := writeFile(t, filepath.Join(src, "ham ad.jpg"), "yeniden adlandır")

	stamp := time.Date(2023, 7, 4, 9, 8, 7, 0, time.Local)
	if err := os.Chtimes(a, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	org := newTestOrganizer(t, dst, false, true)
	org.totalFiles = 1
	if err := org.Process(entryFor(t, a)); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dst, "2023", "07", "04", "20230704_090807.jpg")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("beklenen ad oluşmadı (%s): %v", want, err)
	}
}

func TestResolveConflict_SkipsPlannedPaths(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, filepath.Join(dir, "foto.jpg"), "x")

	org := newTestOrganizer(t, dir, false, false)
	org.plannedPaths[filepath.Join(dir, "foto_1.jpg")] = true

	got, err := org.resolveConflict(target)
	if err != nil {
		t.Fatalf("resolveConflict: %v", err)
	}
	if filepath.Base(got) != "foto_2.jpg" {
		t.Errorf("resolveConflict = %q; planlanmış _1 atlanıp _2 seçilmeliydi", filepath.Base(got))
	}
}

func TestIsSystemDir_UserDirIsSafe(t *testing.T) {
	if isSystemDir(t.TempDir()) {
		t.Error("geçici kullanıcı dizini sistem dizini sayılmamalı")
	}
	if sysRoot := os.Getenv("SystemRoot"); sysRoot != "" && !isSystemDir(sysRoot) {
		t.Errorf("isSystemDir(%q) = false; Windows dizini korumalı olmalıydı", sysRoot)
	}
}

func TestGetMediaDate_FallsBackToModTime(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, filepath.Join(dir, "foto.jpg"), "exif yok")
	stamp := time.Date(2022, 11, 3, 1, 2, 3, 0, time.Local)
	if err := os.Chtimes(a, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	year, month, day, ts := GetMediaDate(a, entryFor(t, a), true)
	if year != "2022" || month != "11" || day != "03" {
		t.Errorf("GetMediaDate = %s/%s/%s; ModTime'a düşmeliydi", year, month, day)
	}
	if ts.Year() != 2022 {
		t.Errorf("dönen zaman damgası ModTime olmalıydı: %v", ts)
	}
}

func TestSupportedFiles_MatchesHelpText(t *testing.T) {
	for _, ext := range []string{".jpg", ".dng", ".mp4", ".3gp"} {
		if _, ok := supportedFiles[ext]; !ok {
			t.Errorf("supportedFiles[%q] eksik", ext)
		}
	}
	if !strings.Contains(AppVersion, "CLI") {
		t.Errorf("AppVersion = %q; CLI sürümünü belirtmeli", AppVersion)
	}
}

func TestProcess_LeavesNoPartialFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	a := writeFile(t, filepath.Join(src, "foto.jpg"), "tamamlanmis kopya")

	org := newTestOrganizer(t, dst, false, false)
	org.totalFiles = 1
	if err := org.Process(entryFor(t, a)); err != nil {
		t.Fatal(err)
	}

	err := filepath.WalkDir(dst, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".lume-part") {
			t.Errorf("yarım kopya dosyası hedefte kaldı: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("hedef taranamadı: %v", err)
	}
}
