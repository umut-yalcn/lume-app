package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log okunamadı: %v", err)
	}
	return string(data)
}

// locateLog, Init'in hangi yolu seçtiğini bulur: önce çalıştırılabilir dosyanın
// dizini denenir, yazılamazsa APPDATA\Lume kullanılır.
func locateLog(t *testing.T) string {
	t.Helper()
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "lume_app.log")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(os.Getenv("APPDATA"), "Lume", "lume_app.log")
}

func startLogger(t *testing.T) string {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	path := locateLog(t)
	t.Cleanup(func() {
		Close()
		os.Remove(path)
		os.Remove(path + ".old")
	})
	return path
}

func TestInit_CreatesLogAndWritesStartMarker(t *testing.T) {
	path := startLogger(t)
	if !strings.Contains(readLog(t, path), "--- Lume Started ---") {
		t.Error("Init başlangıç satırını yazmalıydı")
	}
}

func TestInfoAndError_WriteTaggedLines(t *testing.T) {
	path := startLogger(t)

	Info("arşivlendi: %s", "foto.jpg")
	Error("başarısız: %d", 42)

	content := readLog(t, path)
	if !strings.Contains(content, "[INFO] arşivlendi: foto.jpg") {
		t.Errorf("INFO satırı bulunamadı:\n%s", content)
	}
	if !strings.Contains(content, "[ERROR] başarısız: 42") {
		t.Errorf("ERROR satırı bulunamadı:\n%s", content)
	}
}

func TestSanitizeLogMessage_EscapesLineBreaks(t *testing.T) {
	got := sanitizeLogMessage("kullanıcı girdisi: %s", "satır1\nsatır2\rsatır3")
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("sanitizeLogMessage ham satır sonu bıraktı: %q", got)
	}
	if !strings.Contains(got, `\n`) || !strings.Contains(got, `\r`) {
		t.Errorf("satır sonları kaçırılmış hâlde görünmeli: %q", got)
	}
}

func TestLogInjection_CannotForgeNewLine(t *testing.T) {
	path := startLogger(t)

	Info("dosya: %s", "zararsiz.jpg\n2026/01/01 00:00:00 [ERROR] sahte satır")

	for _, line := range strings.Split(readLog(t, path), "\n") {
		if strings.Contains(line, "sahte satır") && strings.HasPrefix(strings.TrimSpace(line), "2026/01/01") {
			t.Errorf("log injection ile sahte satır üretildi: %q", line)
		}
	}
}

func TestWriteAfterClose_DoesNotPanic(t *testing.T) {
	startLogger(t)
	Close()

	Info("kapandıktan sonra")
	Error("kapandıktan sonra")
	Fatal("kapandıktan sonra")
}

func TestConcurrentWrites_AreSafe(t *testing.T) {
	path := startLogger(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Info("eşzamanlı %d", n)
		}(i)
	}
	wg.Wait()

	content := readLog(t, path)
	if strings.Count(content, "[INFO] eşzamanlı") != 20 {
		t.Errorf("20 eşzamanlı satır beklendi, %d bulundu", strings.Count(content, "[INFO] eşzamanlı"))
	}
}

func TestRotateLog_MovesOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lume_app.log")

	if err := os.WriteFile(path, make([]byte, 6*1024*1024), 0644); err != nil {
		t.Fatal(err)
	}
	rotateLog(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("5 MB üstü log döndürülmeliydi")
	}
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Errorf(".old yedeği oluşmalıydı: %v", err)
	}
}

func TestRotateLog_KeepsSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lume_app.log")

	if err := os.WriteFile(path, []byte("kucuk"), 0644); err != nil {
		t.Fatal(err)
	}
	rotateLog(path)

	if _, err := os.Stat(path); err != nil {
		t.Errorf("küçük log yerinde kalmalıydı: %v", err)
	}
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Error("küçük log için .old üretilmemeliydi")
	}
}
