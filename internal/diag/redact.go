package diag

import (
	"os"
	"os/user"
	"regexp"
	"sort"
	"strings"
)

const (
	redactedToken = "<redacted:token>"
	redactedUser  = "<user>"
	redactedHost  = "<host>"
	homeAlias     = "~"

	// 短い名前（"a" など）を一般の単語ごと置き換えてしまわないための下限
	minIdentityLen = 3
)

type replacement struct {
	re   *regexp.Regexp
	repl string
}

// secretPatterns は、トークンや認証情報に見える文字列の伏せ字化です。
// 完全ではありません（未知の形式は残り得る）。共有する前に、利用者が内容を確認する前提です。
var secretPatterns = []replacement{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), "<redacted:private-key>"},
	{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`), redactedToken},
	{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`), redactedToken},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), redactedToken},
	{regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`), redactedToken},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), "<redacted:jwt>"},
	{regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|basic|token)\s+)[^\s"']+`), "${1}<redacted>"},
	{regexp.MustCompile(`(?i)(://)[^/\s:@]+:[^/\s@]+@`), "${1}<redacted>@"},
	{regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key)\b(\s*[=:]\s*)[^\s"',;]+`), "${1}${2}<redacted>"},
}

// redactor は、保存する文字列から個人情報と認証情報を伏せ字にします。
type redactor struct {
	steps []replacement
}

// newRedactor は、現在のユーザー・ホームディレクトリ・ホスト名に基づく redactor を作ります。
func newRedactor() *redactor {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	host, err := os.Hostname()
	if err != nil {
		host = ""
	}

	username := ""
	if current, err := user.Current(); err == nil {
		username = current.Username
	}

	return newRedactorFor(home, username, host)
}

func newRedactorFor(home, username, host string) *redactor {
	steps := append([]replacement(nil), secretPatterns...)

	// ホームのパスは、ユーザー名を含むため、ユーザー名より先に置き換える。区切り文字の違いも同じ扱いにする。
	for _, variant := range homeVariants(home) {
		steps = append(steps, replacement{regexp.MustCompile(`(?i)` + regexp.QuoteMeta(variant)), homeAlias})
	}

	// "DOMAIN\user" 形式からユーザー名だけを取り出す
	if idx := strings.LastIndexAny(username, `\/`); idx >= 0 {
		username = username[idx+1:]
	}

	if len(username) >= minIdentityLen {
		steps = append(steps, replacement{regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(username) + `\b`), redactedUser})
	}

	if len(host) >= minIdentityLen {
		steps = append(steps, replacement{regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(host) + `\b`), redactedHost})
	}

	return &redactor{steps: steps}
}

// homeVariants はホームのパスの表記ゆれ（\ と /）を、長いものから順に返します。
func homeVariants(home string) []string {
	home = strings.TrimRight(home, `\/`)
	if len(home) < minIdentityLen {
		return nil
	}

	set := map[string]struct{}{
		home:                                {},
		strings.ReplaceAll(home, `\`, `/`):  {},
		strings.ReplaceAll(home, `/`, `\`):  {},
		strings.ReplaceAll(home, `\`, `\\`): {},
	}

	variants := make([]string, 0, len(set))
	for variant := range set {
		variants = append(variants, variant)
	}

	sort.Slice(variants, func(i, j int) bool {
		if len(variants[i]) != len(variants[j]) {
			return len(variants[i]) > len(variants[j])
		}

		return variants[i] < variants[j]
	})

	return variants
}

// redact は文字列の個人情報と認証情報を伏せ字にします。
func (r *redactor) redact(text string) string {
	for _, step := range r.steps {
		text = step.re.ReplaceAllString(text, step.repl)
	}

	return text
}
