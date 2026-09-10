#!/usr/bin/env bash
# Surum notunu uretir: etiket govdesi + yayimlanan ikililerin SHA-256 ozetleri.
#
# Ozetler neden burada hesaplaniyor: ikili dosyalar her yayinda yeniden
# derleniyor ve derleme zaman damgasi gomuyor, yani ayni kaynak koddan
# farkli ozetler cikiyor. Bu yuzden depoda sabit bir ozet listesi tutulamaz;
# liste ancak ikililer uretildikten sonra, burada dogru olabilir.
set -euo pipefail

etiket="${1:?etiket adi gerekli}"
cikti="${2:?cikti dosyasi gerekli}"

# UYARI: Etiket olustururken 'git tag -a --cleanup=verbatim' kullanin.
# Varsayilan temizleme, mesajda '#' ile baslayan satirlari yorum sayip siler;
# Markdown baslikari (## Yenilikler gibi) bu yuzden sessizce kaybolur ve
# surum notu basliksiz yayimlanir.
git fetch --force --tags origin "refs/tags/${etiket}:refs/tags/${etiket}"
git tag -l --format='%(contents:body)' "${etiket}" > "${cikti}"

{
    echo
    echo "## Dosya özetleri (SHA-256)"
    echo
    echo "| Dosya | SHA-256 |"
    echo "|---|---|"
} >> "${cikti}"

for yol in "python version lume/dist/lume_python_gui.exe" \
           "go version lume/lume_go_gui.exe" \
           "cli version lume/lume_go_cli.exe"; do
    if [ ! -f "${yol}" ]; then
        echo "uyari: ${yol} bulunamadi, ozet atlandi" >&2
        continue
    fi
    ad="$(basename "${yol}")"
    ozet="$(sha256sum "${yol}" | cut -d' ' -f1)"
    printf '| `%s` | `%s` |\n' "${ad}" "${ozet}" >> "${cikti}"
done

{
    echo
    echo "İndirdiğiniz dosyayı doğrulamak için PowerShell'de:"
    echo
    echo '```powershell'
    echo 'Get-FileHash .\lume_go_cli.exe -Algorithm SHA256'
    echo '```'
} >> "${cikti}"

echo "surum notu yazildi: ${cikti}"
