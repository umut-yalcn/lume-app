import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from exif_reader import (
    LONG_PATH_THRESHOLD,
    detect_source,
    get_file_hash,
    get_file_info,
    long_path,
)
from file_organizer import (
    archive_file,
    calculate_new_path,
    handle_conflict,
    is_nested,
    sanitize_folder_name,
)


class SanitizeFolderNameTest(unittest.TestCase):

    def test_dot_names_become_unknown(self):
        self.assertEqual(sanitize_folder_name("."), "Unknown")
        self.assertEqual(sanitize_folder_name(".."), "Unknown")

    def test_traversal_is_neutralised(self):
        self.assertNotIn("..", sanitize_folder_name("a..b"))

    def test_reserved_device_names(self):
        for name in ("CON", "COM1", "LPT9", "NUL"):
            self.assertTrue(sanitize_folder_name(name).endswith("_safe"), name)

    def test_length_is_capped_without_breaking_characters(self):
        result = sanitize_folder_name("ğ" * 150)
        self.assertLessEqual(len(result), 100)
        self.assertEqual(result, "ğ" * 100)

    def test_empty_input_falls_back(self):
        self.assertEqual(sanitize_folder_name("   "), "Unknown")


class IsNestedTest(unittest.TestCase):

    def test_identical_paths(self):
        self.assertTrue(is_nested(r"C:\a", r"C:\a"))

    def test_target_inside_source(self):
        self.assertTrue(is_nested(r"C:\a", r"C:\a\b"))

    def test_source_inside_target(self):
        self.assertTrue(is_nested(r"C:\a\b", r"C:\a"))

    def test_siblings_are_not_nested(self):
        self.assertFalse(is_nested(r"C:\a", r"C:\b"))

    def test_prefix_is_not_nesting(self):
        self.assertFalse(is_nested(r"C:\arsiv", r"C:\arsiv2"))


class ArchiveFileTest(unittest.TestCase):

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.src = os.path.join(self.tmp.name, "src")
        self.dst = os.path.join(self.tmp.name, "dst")
        os.makedirs(self.src)
        os.makedirs(self.dst)

    def tearDown(self):
        self.tmp.cleanup()

    def _write(self, name, content):
        path = os.path.join(self.src, name)
        with open(path, "wb") as handle:
            handle.write(content)
        return path

    def _archived(self):
        return sorted(
            os.path.relpath(os.path.join(root, name), self.dst)
            for root, _, names in os.walk(self.dst)
            for name in names
        )

    def test_source_is_preserved(self):
        path = self._write("IMG_001.jpg", b"orijinal")
        self.assertTrue(archive_file(get_file_info(path), self.dst))
        self.assertTrue(
            os.path.exists(path),
            "arşivleme kopyalamalı, kaynak dosya silinmemeli",
        )
        self.assertEqual(len(self._archived()), 1)

    def test_content_is_copied_intact(self):
        path = self._write("IMG_002.jpg", b"bozulmamis icerik")
        archive_file(get_file_info(path), self.dst)
        copied = os.path.join(self.dst, self._archived()[0])
        with open(copied, "rb") as handle:
            self.assertEqual(handle.read(), b"bozulmamis icerik")

    def test_duplicate_is_not_copied_twice(self):
        path = self._write("IMG_003.jpg", b"ayni")
        archive_file(get_file_info(path), self.dst)
        first = self._archived()

        self.assertTrue(archive_file(get_file_info(path), self.dst))
        self.assertEqual(self._archived(), first)

    def test_same_name_different_content_gets_suffix(self):
        path = self._write("IMG_004.jpg", b"birinci")
        archive_file(get_file_info(path), self.dst)

        with open(path, "wb") as handle:
            handle.write(b"ikinci ve farkli")
        archive_file(get_file_info(path), self.dst)

        names = [os.path.basename(item) for item in self._archived()]
        self.assertIn("IMG_004.jpg", names)
        self.assertIn("IMG_004_1.jpg", names)

    def test_no_partial_file_remains_after_archive(self):
        path = self._write("IMG_010.jpg", b"tamamlanmis kopya")
        self.assertTrue(archive_file(get_file_info(path), self.dst))

        artik = [
            os.path.join(kok, ad)
            for kok, _, adlar in os.walk(self.dst)
            for ad in adlar
            if ad.endswith(".lume-part")
        ]
        self.assertEqual(artik, [], "yarım kopya dosyası hedefte kaldı")

    def test_missing_source_is_reported(self):
        info = get_file_info(self._write("IMG_005.jpg", b"gecici"))
        os.remove(info["path"])
        self.assertFalse(archive_file(info, self.dst))

    def test_target_path_stays_under_base(self):
        path = self._write("IMG_006.jpg", b"yol")
        info = get_file_info(path)
        target = calculate_new_path(info, self.dst)
        self.assertTrue(
            os.path.abspath(target).startswith(os.path.abspath(self.dst) + os.sep)
        )


class ConflictTest(unittest.TestCase):

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.tmp.cleanup()

    def test_free_path_is_returned_unchanged(self):
        target = os.path.join(self.tmp.name, "yeni.jpg")
        source = os.path.join(self.tmp.name, "kaynak.jpg")
        with open(source, "wb") as handle:
            handle.write(b"x")
        self.assertEqual(handle_conflict(source, target), (target, False))

    def test_identical_file_is_flagged_as_duplicate(self):
        source = os.path.join(self.tmp.name, "kaynak.jpg")
        target = os.path.join(self.tmp.name, "hedef.jpg")
        for path in (source, target):
            with open(path, "wb") as handle:
                handle.write(b"ayni icerik")
        self.assertEqual(handle_conflict(source, target), (target, True))


class HashAndSourceTest(unittest.TestCase):

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.tmp.cleanup()

    def test_full_hash_matches_for_identical_content(self):
        first = os.path.join(self.tmp.name, "a.jpg")
        second = os.path.join(self.tmp.name, "b.jpg")
        for path in (first, second):
            with open(path, "wb") as handle:
                handle.write(b"esit")
        self.assertEqual(
            get_file_hash(first, quick=False), get_file_hash(second, quick=False)
        )

    def test_missing_file_returns_empty_hash(self):
        self.assertEqual(
            get_file_hash(os.path.join(self.tmp.name, "yok.jpg"), quick=False), ""
        )

    def test_source_detection(self):
        self.assertEqual(detect_source("IMG_20240101.jpg"), "Camera")
        self.assertEqual(detect_source("WhatsApp Image.jpg"), "WhatsApp")
        self.assertEqual(detect_source("screenshot_1.png"), "Screenshots")
        self.assertIsNone(detect_source("rastgele.jpg"))


class LongPathTest(unittest.TestCase):

    def setUp(self):
        if os.name != "nt":
            self.skipTest("uzun yol öneki yalnız Windows'ta uygulanır")

    def test_short_path_is_returned_absolute_without_prefix(self):
        result = long_path("kisa.jpg")
        self.assertFalse(result.startswith(r"\\?"))
        self.assertTrue(os.path.isabs(result))

    def test_long_path_gets_extended_prefix(self):
        deep = "C:" + os.sep + "a" * (LONG_PATH_THRESHOLD + 20) + os.sep + "foto.jpg"
        self.assertTrue(long_path(deep).startswith("\\\\?\\"))

    def test_long_unc_path_gets_unc_prefix(self):
        share = r"\\sunucu\paylasim" + "\\" + "b" * (LONG_PATH_THRESHOLD + 20) + r"\x.jpg"
        self.assertTrue(long_path(share).startswith("\\\\?\\UNC\\"))

    def test_already_prefixed_path_is_untouched(self):
        prefixed = "\\\\?\\C:\\zaten\\onekli.jpg"
        self.assertEqual(long_path(prefixed), prefixed)

    def test_deep_directory_is_archived_end_to_end(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)

        deep = tmp.name
        while len(deep) < LONG_PATH_THRESHOLD + 40:
            deep = os.path.join(deep, "klasor_" + "u" * 20)
        os.makedirs(long_path(deep), exist_ok=True)

        source = os.path.join(deep, "IMG_900.jpg")
        with open(long_path(source), "wb") as handle:
            handle.write(b"derin klasordeki dosya")

        self.assertGreater(len(source), LONG_PATH_THRESHOLD)

        info = get_file_info(source)
        self.assertTrue(info, "uzun yoldaki dosya okunabilmeliydi")

        target = os.path.join(tmp.name, "hedef")
        os.makedirs(target, exist_ok=True)
        self.assertTrue(archive_file(info, target))

        archived = [
            name
            for _, _, names in os.walk(target)
            for name in names
        ]
        self.assertIn("IMG_900.jpg", archived)


class QuickHashCollisionTest(unittest.TestCase):

    def test_quick_hash_matches_for_same_size_mtime_and_header(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)

        first = os.path.join(tmp.name, "a.jpg")
        second = os.path.join(tmp.name, "b.jpg")
        header = b"H" * 4096
        with open(first, "wb") as handle:
            handle.write(header + b"birinci kuyruk")
        with open(second, "wb") as handle:
            handle.write(header + b"ikinci kuyrukk")

        os.utime(first, (1700000000, 1700000000))
        os.utime(second, (1700000000, 1700000000))

        self.assertEqual(
            get_file_hash(first, quick=True),
            get_file_hash(second, quick=True),
            "bu senaryo hızlı karma çakışmasını temsil etmeli",
        )
        self.assertNotEqual(
            get_file_hash(first, quick=False),
            get_file_hash(second, quick=False),
            "tam karma iki dosyayı ayırt etmeli",
        )


if __name__ == "__main__":
    unittest.main(verbosity=2)
