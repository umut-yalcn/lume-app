import importlib
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from file_organizer import archive_file, calculate_new_path, sanitize_folder_name
from exif_reader import get_exif_data, get_file_info


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
