package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// isolate, kayit yolunu testler arasinda temiz tutar: getConfigPath once
// calistirilabilir dosyanin dizinini dener, bu yuzden testler ayni dosyayi paylasabilir.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	path := getConfigPath()
	os.Remove(path)
	os.Remove(path + ".tmp")
	t.Cleanup(func() {
		os.Remove(path)
		os.Remove(path + ".tmp")
	})
}

func TestLoadConfig_MissingFileFallsBackToDefaults(t *testing.T) {
	isolate(t)

	conf := LoadConfig()
	if conf.Language != "tr" {
		t.Errorf("Language = %q; kayıt yokken varsayılan 'tr' olmalı", conf.Language)
	}
	if conf.DarkMode || conf.DryRun || conf.Rename {
		t.Error("kayıt yokken tüm bayraklar kapalı başlamalı")
	}
}

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	isolate(t)

	want := Config{
		DarkMode:     true,
		Language:     "en",
		TargetFolder: filepath.Join(dir, "arsiv"),
		DryRun:       true,
		Rename:       true,
		Stats:        Stats{TotalFiles: 12, TotalSize: 3456, TotalOrganized: 2},
	}
	if err := SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got := LoadConfig()
	if got != want {
		t.Errorf("LoadConfig() = %+v; kaydedilen değerle aynı olmalıydı %+v", got, want)
	}
}

func TestLoadConfig_RejectsUnknownLanguage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	isolate(t)

	conf := Config{Language: "de", DarkMode: true}
	if err := SaveConfig(conf); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got := LoadConfig()
	if got.Language != "tr" {
		t.Errorf("Language = %q; desteklenmeyen dil 'tr'ye düşmeliydi", got.Language)
	}
	if !got.DarkMode {
		t.Error("dil düzeltilirken diğer alanlar korunmalıydı")
	}
}

func TestLoadConfig_CorruptFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	isolate(t)

	path := getConfigPath()
	if err := os.WriteFile(path, []byte("{bozuk json"), 0644); err != nil {
		t.Fatalf("bozuk dosya yazılamadı: %v", err)
	}

	got := LoadConfig()
	if got.Language != "tr" || got.DarkMode {
		t.Errorf("LoadConfig() = %+v; bozuk kayıtta varsayılana dönmeliydi", got)
	}
}

func TestSaveConfig_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	isolate(t)

	if err := SaveConfig(Config{Language: "tr"}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	path := getConfigPath()
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("başarılı kayıttan sonra .tmp dosyası kalmamalı")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("kayıt okunamadı: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Errorf("kayıt geçerli JSON olmalı: %v", err)
	}
}
