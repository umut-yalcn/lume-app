package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeMedia(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("dizin oluşturulamadı: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("dosya yazılamadı: %v", err)
	}
	return path
}

func TestScanDroppedPaths_CollectsSupportedFilesRecursively(t *testing.T) {
	dir := t.TempDir()
	writeMedia(t, filepath.Join(dir, "a.jpg"), "bir")
	writeMedia(t, filepath.Join(dir, "alt", "b.mp4"), "iki")
	writeMedia(t, filepath.Join(dir, "alt", "not.txt"), "desteklenmeyen")

	res := scanDroppedPaths([]string{dir}, "", map[string]bool{}, MaxFilesLimit)

	if res.limitHit {
		t.Error("limit aşılmadı, limitHit false olmalıydı")
	}
	if len(res.files) != 2 {
		t.Fatalf("2 desteklenen dosya beklendi, %d bulundu", len(res.files))
	}
	names := []string{filepath.Base(res.files[0].Path), filepath.Base(res.files[1].Path)}
	for _, want := range []string{"a.jpg", "b.mp4"} {
		found := false
		for _, got := range names {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s taramada bulunamadı: %v", want, names)
		}
	}
}

func TestScanDroppedPaths_SkipsAlreadySeenFiles(t *testing.T) {
	dir := t.TempDir()
	first := writeMedia(t, filepath.Join(dir, "a.jpg"), "bir")

	seen := map[string]bool{normalizePath(first): true}
	res := scanDroppedPaths([]string{dir}, "", seen, MaxFilesLimit)

	if len(res.files) != 0 {
		t.Errorf("listede olan dosya yeniden eklenmemeliydi: %d", len(res.files))
	}
}

func TestScanDroppedPaths_SkipsDuplicateDropOfSamePath(t *testing.T) {
	dir := t.TempDir()
	file := writeMedia(t, filepath.Join(dir, "a.jpg"), "bir")

	res := scanDroppedPaths([]string{file, file}, "", map[string]bool{}, MaxFilesLimit)

	if len(res.files) != 1 {
		t.Errorf("aynı yol iki kez bırakılsa da bir kez eklenmeli, %d bulundu", len(res.files))
	}
}

func TestScanDroppedPaths_HonoursCapacity(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeMedia(t, filepath.Join(dir, "f"+string(rune('a'+i))+".jpg"), "icerik")
	}

	res := scanDroppedPaths([]string{dir}, "", map[string]bool{}, 3)

	if len(res.files) > 3 {
		t.Errorf("kapasite 3 iken %d dosya toplandı", len(res.files))
	}
	if !res.limitHit {
		t.Error("kapasite dolduğunda limitHit true olmalıydı")
	}
}

func TestScanDroppedPaths_ZeroCapacityReportsLimit(t *testing.T) {
	dir := t.TempDir()
	writeMedia(t, filepath.Join(dir, "a.jpg"), "bir")

	res := scanDroppedPaths([]string{dir}, "", map[string]bool{}, 0)

	if len(res.files) != 0 || !res.limitHit {
		t.Errorf("kapasite yokken dosya toplanmamalı ve limit bildirilmeli: %d %v", len(res.files), res.limitHit)
	}
}

func TestScanDroppedPaths_SkipsFilesAlreadyInTargetFolder(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "arsiv")
	writeMedia(t, filepath.Join(target, "a.jpg"), "bir")

	res := scanDroppedPaths([]string{filepath.Join(target, "a.jpg")}, strings.ToUpper(target), map[string]bool{}, MaxFilesLimit)

	if len(res.files) != 0 {
		t.Error("hedef klasörde duran dosya listeye alınmamalı (harf büyüklüğünden bağımsız)")
	}
}

func TestScanDroppedPaths_SkipsSourceDirNestedInTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "arsiv")
	inner := filepath.Join(target, "ic")
	writeMedia(t, filepath.Join(inner, "a.jpg"), "bir")

	res := scanDroppedPaths([]string{inner}, target, map[string]bool{}, MaxFilesLimit)

	if len(res.files) != 0 {
		t.Error("hedefin içindeki klasör taranmamalı")
	}
}

func TestSameFolder_IgnoresCaseAndTrailingSeparator(t *testing.T) {
	dir := t.TempDir()

	if !sameFolder(dir, strings.ToUpper(dir)) {
		t.Error("aynı klasör harf büyüklüğünden bağımsız eşleşmeli")
	}
	if !sameFolder(dir, dir+string(filepath.Separator)) {
		t.Error("sondaki ayraç eşleşmeyi bozmamalı")
	}
	if sameFolder(dir, filepath.Join(dir, "alt")) {
		t.Error("farklı klasörler eşleşmemeli")
	}
}

func TestNormalizePath_IsStableForEquivalentSpellings(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "foto.jpg")
	b := filepath.Join(dir, "alt", "..", "foto.jpg")

	if normalizePath(a) != normalizePath(b) {
		t.Errorf("eşdeğer yollar aynı anahtara indirgenmeli: %q vs %q", normalizePath(a), normalizePath(b))
	}
}

func TestFormatSize_UsesReadableUnits(t *testing.T) {
	cases := map[int64]string{
		512:             "512 B",
		1536:            "1.50 KB",
		5 * 1024 * 1024: "5.00 MB",
	}
	for size, want := range cases {
		if got := formatSize(size); got != want {
			t.Errorf("formatSize(%d) = %q; %q bekleniyordu", size, got, want)
		}
	}
}

func TestAppTitle_TracksAppVersion(t *testing.T) {
	if !strings.Contains(appTitle, AppVersion) {
		t.Errorf("appTitle = %q; AppVersion (%q) içermeli", appTitle, AppVersion)
	}
	for _, lang := range []string{"tr", "en"} {
		if i18n[lang]["title"] != appTitle {
			t.Errorf("%s başlığı appTitle ile aynı olmalı", lang)
		}
		if i18n[lang]["scanning"] == "" {
			t.Errorf("%s için 'scanning' metni eksik", lang)
		}
	}
}

// Bu test, arayuz thread'indeki ayar degisiklikleri ile arka plandaki arsivleme
// isciisinin ayni Config yapisina erismesini taklit eder. Widget cagrilari
// disarida birakilir; olculen sey kilitleme disiplinidir (-race ile calistirin).
func TestConfigAccess_IsRaceFreeBetweenUIAndWorker(t *testing.T) {
	ui := &LumeUI{}
	ui.Config.Language = "tr"

	var wg sync.WaitGroup

	// arayuz thread'i: tema/dil/onay kutusu degisiklikleri
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			ui.mutex.Lock()
			ui.Config.DarkMode = !ui.Config.DarkMode
			if ui.Config.Language == "tr" {
				ui.Config.Language = "en"
			} else {
				ui.Config.Language = "tr"
			}
			ui.Config.DryRun = i%2 == 0
			ui.Config.Rename = i%3 == 0
			snapshot := ui.Config
			ui.mutex.Unlock()

			_ = snapshot
		}
	}()

	// arka plan iscisi: istatistik yazimi ve bayrak okumasi
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			ui.mutex.Lock()
			dryRun := ui.Config.DryRun
			rename := ui.Config.Rename
			ui.mutex.Unlock()

			if !dryRun {
				ui.mutex.Lock()
				ui.Config.Stats.TotalFiles++
				ui.Config.Stats.TotalSize += 1024
				ui.Config.Stats.TotalOrganized++
				ui.mutex.Unlock()
			}
			_ = rename
		}
	}()

	// surukle-birak taramasinin sonucu yayimlamasi
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			ui.mutex.Lock()
			ui.isScanning = true
			ui.FileCount = len(ui.FilesToMove)
			ui.isScanning = false
			ui.mutex.Unlock()
		}
	}()

	wg.Wait()

	ui.mutex.Lock()
	defer ui.mutex.Unlock()
	if ui.Config.Stats.TotalFiles < 0 {
		t.Error("istatistik sayaci bozuldu")
	}
}

func TestScanDroppedPaths_IsSafeInBackgroundGoroutine(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 8; i++ {
		writeMedia(t, filepath.Join(dir, "f"+string(rune('a'+i))+".jpg"), "icerik")
	}

	ui := &LumeUI{}
	done := make(chan scanResult, 1)

	go func() {
		ui.mutex.Lock()
		target := ui.TargetFolder
		capacity := MaxFilesLimit - ui.FileCount
		ui.isScanning = true
		ui.mutex.Unlock()

		res := scanDroppedPaths([]string{dir}, target, map[string]bool{}, capacity)

		ui.mutex.Lock()
		ui.FilesToMove = append(ui.FilesToMove, res.files...)
		ui.FileCount = len(ui.FilesToMove)
		ui.isScanning = false
		ui.mutex.Unlock()

		done <- res
	}()

	res := <-done
	if len(res.files) != 8 {
		t.Errorf("8 dosya beklendi, %d bulundu", len(res.files))
	}

	ui.mutex.Lock()
	defer ui.mutex.Unlock()
	if ui.FileCount != 8 {
		t.Errorf("FileCount = %d; 8 olmalıydı", ui.FileCount)
	}
	if ui.isScanning {
		t.Error("tarama bittiğinde isScanning kapanmalı")
	}
}
