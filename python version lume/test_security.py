import importlib
import os
import shutil
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from file_organizer import archive_file, calculate_new_path, sanitize_folder_name
from exif_reader import get_exif_data, get_file_hash, get_file_info, is_link


class SanitizeSecurityTest(unittest.TestCase):

    def test_no_separators_traversal_or_control_chars(self):
        attacks = [
            r"..\..\..\Windows\System32",
            "../../../../etc/passwd",
            "....//....//evil",
            "foo/../../bar",
            r"\\?\C:\Windows",
            "a\x00b",
            "../" * 40 + "pwn",
            ".", "..", "   ", "con", "PRN", "NUL", "COM1", "LPT9",
            "name\x01\x1f",
        ]
        for attack in attacks:
            got = sanitize_folder_name(attack)
            self.assertNotIn("/", got, f"{attack!r} -> {got!r}")
            self.assertNotIn("\\", got, f"{attack!r} -> {got!r}")
            self.assertNotIn("..", got, f"{attack!r} -> {got!r}")
            self.assertNotIn("\x00", got, f"{attack!r} -> {got!r}")
            self.assertNotIn(got, (".", "..", ""), f"{attack!r} -> {got!r}")


class ArchiveEscapeSecurityTest(unittest.TestCase):

    def setUp(self):
        self.base = tempfile.mkdtemp()
        self.target = os.path.join(self.base, "arsiv")
        os.makedirs(self.target)

    def test_evil_device_name_stays_under_target(self):
        src = os.path.join(self.base, "k.jpg")
        with open(src, "wb") as handle:
            handle.write(b"zararsiz")

        for device in [r"..\..\DISARIDA", "../../DISARIDA", "../" * 10 + "DISARIDA"]:
            info = {
                "path": src, "filename": "k.jpg", "year": "2024", "month": "01",
                "device": device, "source": None, "date": None, "date_str": "",
            }
            target_path = os.path.abspath(calculate_new_path(info, self.target))
            self.assertTrue(
                target_path.startswith(os.path.abspath(self.target) + os.sep),
                f"KAÇIŞ: {device!r} -> {target_path}",
            )

        self.assertFalse(os.path.exists(os.path.join(self.base, "DISARIDA")))

    def test_archive_does_not_write_outside_base(self):
        src = os.path.join(self.base, "k.jpg")
        with open(src, "wb") as handle:
            handle.write(b"x")
        info = get_file_info(src)
        info["device"] = "../../DISARIDA"
        archive_file(info, self.target)
        self.assertFalse(os.path.exists(os.path.join(self.base, "DISARIDA")))


class ShortPathRegressionTest(unittest.TestCase):
    """8.3 kısa ad (RUNNER~1) içeren yollar symlink sanılmamalı.

    Eski kod realpath ile abspath'i karşılaştırıyordu; kısa ad içeren her
    yolda sıradan dosyalar sessizce reddediliyordu.
    """

    def _kisa_ad(self, klasor):
        if os.name != "nt":
            self.skipTest("8.3 kısa ad yalnız Windows'ta")
        try:
            sonuc = subprocess.run(
                ["powershell", "-NoProfile", "-Command",
                 f"(New-Object -ComObject Scripting.FileSystemObject)"
                 f".GetFolder('{klasor}').ShortPath"],
                capture_output=True, text=True, timeout=30)
        except (OSError, subprocess.SubprocessError):
            self.skipTest("kısa ad alınamadı")
        kisa = sonuc.stdout.strip()
        if not kisa or "~" not in kisa:
            self.skipTest("dosya sistemi 8.3 kısa ad üretmiyor")
        return kisa

    def test_short_name_path_is_not_treated_as_symlink(self):
        base = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base, True)

        uzun = os.path.join(base, "runneradmin_uzun_klasor_adi")
        os.makedirs(uzun, exist_ok=True)
        kaynak = os.path.join(uzun, "IMG_001.jpg")
        with open(kaynak, "wb") as handle:
            handle.write(b"icerik")

        kisa_yol = os.path.join(self._kisa_ad(uzun), "IMG_001.jpg")

        self.assertFalse(is_link(kisa_yol), "kısa ad symlink değildir")
        self.assertTrue(get_file_info(kisa_yol), "kısa adlı yol okunabilmeli")
        self.assertTrue(get_file_hash(kisa_yol, quick=False),
                        "kısa adlı yolda karma hesaplanabilmeli")

    def test_short_name_file_can_be_archived(self):
        base = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base, True)

        uzun = os.path.join(base, "runneradmin_uzun_klasor_adi")
        os.makedirs(uzun, exist_ok=True)
        kaynak = os.path.join(uzun, "IMG_002.jpg")
        with open(kaynak, "wb") as handle:
            handle.write(b"arsivlenecek")

        kisa_yol = os.path.join(self._kisa_ad(uzun), "IMG_002.jpg")
        hedef = os.path.join(base, "arsiv")
        os.makedirs(hedef, exist_ok=True)

        info = get_file_info(kisa_yol)
        self.assertTrue(info)
        self.assertTrue(archive_file(info, hedef), "kısa adlı dosya arşivlenebilmeli")

        arsivlenen = [ad for _, _, adlar in os.walk(hedef) for ad in adlar]
        self.assertIn("IMG_002.jpg", arsivlenen)


class SymlinkSecurityTest(unittest.TestCase):

    def test_symlink_source_is_blocked(self):
        base = tempfile.mkdtemp()
        real = os.path.join(base, "gercek.jpg")
        with open(real, "wb") as handle:
            handle.write(b"gizli")
        link = os.path.join(base, "link.jpg")
        try:
            os.symlink(real, link)
        except (OSError, NotImplementedError):
            self.skipTest("symlink oluşturulamadı (yetki yok)")
        self.assertEqual(get_file_info(link), {}, "symlink kaynak engellenmedi")


class ConfigPoisoningTest(unittest.TestCase):

    def _fresh_config(self):
        os.environ["APPDATA"] = tempfile.mkdtemp()
        import config_manager
        importlib.reload(config_manager)
        return config_manager

    def test_unknown_keys_are_dropped(self):
        cm = self._fresh_config()
        cm.save_config({
            "language": "tr", "appearance_mode": "dark",
            "__proto__": "x", "evil": "rm -rf", "cmd": "calc.exe",
        })
        loaded = cm.load_config()
        for key in ("evil", "cmd", "__proto__"):
            self.assertNotIn(key, loaded, f"zehirli anahtar korundu: {key}")

    def test_config_with_bom_is_still_read(self):
        """Not Defteri "UTF-8" ile kaydedince dosyaya BOM ekler."""
        cm = self._fresh_config()
        os.makedirs(os.path.dirname(cm.CONFIG_FILE), exist_ok=True)
        with open(cm.CONFIG_FILE, "w", encoding="utf-8-sig") as handle:
            handle.write('{"language": "tr", "appearance_mode": "dark"}')

        loaded = cm.load_config()
        self.assertEqual(loaded["language"], "tr", "BOM'lu kayıt okunmalıydı")
        self.assertEqual(loaded["appearance_mode"], "dark")

    def test_invalid_values_are_rejected(self):
        cm = self._fresh_config()
        cm.save_config({"language": "'; DROP TABLE", "appearance_mode": "evil"})
        loaded = cm.load_config()
        self.assertIn(loaded["language"], ("tr", "en"))
        self.assertIn(loaded["appearance_mode"], ("dark", "light"))


class ExifDoSTest(unittest.TestCase):

    def test_malformed_exif_does_not_crash(self):
        directory = tempfile.mkdtemp()
        payloads = {
            "bos.jpg": b"",
            "yarim.jpg": b"\xff\xd8\xff\xe1\x00\x10Exif\x00\x00",
            "cop.jpg": b"\xff\xe1\xff\xff" * 5000,
            "buyuk.jpg": (
                b"\xff\xd8\xff\xe1\xff\xfeExif\x00\x00II*\x00\x08\x00\x00\x00"
                + b"A" * 100000
            ),
        }
        for name, data in payloads.items():
            path = os.path.join(directory, name)
            with open(path, "wb") as handle:
                handle.write(data)
            try:
                get_exif_data(path)
            except Exception as error:  # noqa: BLE001 - DoS kanıtı
                self.fail(f"{name} EXIF DoS: {error}")


class LogInjectionSecurityTest(unittest.TestCase):

    def test_newline_in_message_is_escaped(self):
        directory = tempfile.mkdtemp()
        os.environ["APPDATA"] = directory
        import logger_config
        importlib.reload(logger_config)

        forged_tail = "2026-01-01 00:00:00 - ERROR - SAHTE"
        logger_config.logger.info("Dropped: %s", "ok.jpg\n" + forged_tail)

        content = open(os.path.join(directory, "Lume", "app.log"), encoding="utf-8").read()
        forged = [
            line for line in content.splitlines()
            if line.strip().startswith("2026-01-01") and "SAHTE" in line
        ]
        self.assertEqual(forged, [], "log injection ile sahte satır üretildi")
        self.assertIn("\\n", content, "satır sonu kaçırılmış görünmeli")


if __name__ == "__main__":
    unittest.main(verbosity=2)
