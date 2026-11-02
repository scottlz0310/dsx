package updater

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// 判定不能の疑い（Suspect）の検知は、出力を正しく解釈できなかった可能性を知らせるだけです。
// 更新の実行・件数・結果は補正しません。更新の兆候があるのに「更新なし」と成功扱いになる、
// といった見落としを、利用者と Arbor が気づける形にするためのものです。

// corruptedPackageReason は解析結果に文字コードの崩れた文字列があるとき、その理由を返します。
// 全角文字の桁ずれなどでバイト列を途中で切ると、不正な UTF-8 が混入します。
func corruptedPackageReason(packages []PackageInfo) string {
	for i, pkg := range packages {
		for _, text := range []string{pkg.Name, pkg.CurrentVersion, pkg.NewVersion} {
			if !utf8.ValidString(text) || strings.ContainsRune(text, utf8.RuneError) {
				return fmt.Sprintf("解析結果の %d 件目に、文字コードの崩れた文字列が含まれています（出力の解釈を誤っている可能性）", i+1)
			}
		}
	}

	return ""
}
