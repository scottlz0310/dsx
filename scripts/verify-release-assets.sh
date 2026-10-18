#!/bin/sh
# Usage: verify-release-assets.sh <asset-dir> <version> [--with-msix]
# draft Release から再取得した asset が揃っていて改ざん・欠落がないことを確認する。
#   - 全プラットフォームのアーカイブと checksums.txt があり、checksum が一致する
#   - --with-msix: MSIX 一式（署名済み MSIX・App Installer・公開証明書・install.ps1）があり、版が一致する
#   - Linux x86_64 では、取り出したバイナリを実行して版を確認する
# <version> は先頭に v を付けない（例: 0.11.0）。
set -eu

with_msix=false
case "$#:${3:-}" in
    2:) ;;
    3:--with-msix) with_msix=true ;;
    *)
        echo 'Usage: verify-release-assets.sh <asset-dir> <version> [--with-msix]' >&2
        exit 2
        ;;
esac
dir=$1
version=$2

fail() {
    echo "::error::$1" >&2
    exit 1
}

require_file() {
    [ -s "$dir/$1" ] || fail "asset が無いか空です: $1"
}

archives="
dsx_${version}_linux_amd64.tar.gz
dsx_${version}_linux_arm64.tar.gz
dsx_${version}_darwin_amd64.tar.gz
dsx_${version}_darwin_arm64.tar.gz
dsx_${version}_windows_amd64.zip
"

require_file checksums.txt
for archive in $archives; do
    require_file "$archive"
    grep -q " $archive\$" "$dir/checksums.txt" || fail "checksums.txt に記載がありません: $archive"
done
(cd "$dir" && sha256sum -c checksums.txt) || fail 'checksum が一致しません'

if [ "$with_msix" = true ]; then
    for file in dsx_x64.msix dsx.appinstaller dsx.cer install.ps1; do
        require_file "$file"
    done
    grep -q "releases/download/v${version}/dsx_x64.msix" "$dir/dsx.appinstaller" ||
        fail "dsx.appinstaller の MSIX URL が v${version} を指していません"
    grep -q "Version=\"${version}.0\"" "$dir/dsx.appinstaller" ||
        fail "dsx.appinstaller の Version が ${version}.0 ではありません"
fi

if [ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ]; then
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    tar -xzf "$dir/dsx_${version}_linux_amd64.tar.gz" -C "$work"
    output=$("$work/dsx" --version) || fail 'dsx --version が失敗しました'
    case "$output" in
        *"$version"*) ;;
        *) fail "dsx --version の出力が版 ${version} を含みません: $output" ;;
    esac
fi

echo "Release assets verified for ${version}."
