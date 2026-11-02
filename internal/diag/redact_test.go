package diag

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactor_Secrets(t *testing.T) {
	r := newRedactorFor("", "", "")

	token36 := strings.Repeat("a", 36)

	// ダミーの値。秘密情報のスキャナ（gitleaks）が実物と誤検知しないよう、リテラルを分割して組み立てる
	awsKey := "AKIA" + "ABCDEFGHIJKLMNOP"
	jwt := "eyJhbGciOiJIUzI1NiJ9" + "." + "eyJzdWIiOiIxMjM0NTY3ODkwIn0" + "." + "abcdefghijklmnop"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"GitHub トークン", "token ghp_" + token36 + " end", "token <redacted:token> end"},
		{"GitHub fine-grained", "x github_pat_" + strings.Repeat("A1_", 10) + " y", "x <redacted:token> y"},
		{"AWS アクセスキー", "key=" + awsKey, "key=<redacted:token>"},
		{"Slack トークン", "xoxb-1234567890-abcdef", "<redacted:token>"},
		{"Authorization ヘッダー", "Authorization: Bearer abc.def-123", "Authorization: Bearer <redacted>"},
		{"URL の認証情報", "https://user:s3cret@example.com/path", "https://<redacted>@example.com/path"},
		{"password=", "password=hunter2 next", "password=<redacted> next"},
		{"api_key: ", "API_KEY: abcdef123", "API_KEY: <redacted>"},
		{"JWT", jwt, "<redacted:jwt>"},
		{"秘密鍵", "a\n-----BEGIN RSA PRIVATE KEY-----\nMIIB\n-----END RSA PRIVATE KEY-----\nb", "a\n<redacted:private-key>\nb"},
		{"通常の出力は変えない", "Everything is ok!\nfd 10.2.0 10.5.0", "Everything is ok!\nfd 10.2.0 10.5.0"},
		{"空文字列", "", ""},
		{"短い ghp_ は対象外", "ghp_short", "ghp_short"},
		{"secrets という語は対象外", "secrets: managed", "secrets: managed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.redact(tt.in))
		})
	}
}

func TestRedactor_Identity(t *testing.T) {
	tests := []struct {
		name     string
		home     string
		username string
		host     string
		in       string
		want     string
	}{
		{
			name: "Windows のホーム（バックスラッシュ）",
			home: `C:\Users\alice`, username: `DESKTOP\alice`, host: "devbox",
			in:   `C:\Users\alice\scoop\apps\fd`,
			want: `~\scoop\apps\fd`,
		},
		{
			name: "Windows のホーム（スラッシュ表記）",
			home: `C:\Users\alice`, username: "alice", host: "devbox",
			in:   "C:/Users/alice/scoop",
			want: "~/scoop",
		},
		{
			name: "大文字小文字を区別しない（Windows のパス）",
			home: `C:\Users\alice`, username: "alice", host: "devbox",
			in:   `c:\users\ALICE\scoop`,
			want: `~\scoop`,
		},
		{
			name: "エスケープされたバックスラッシュ",
			home: `C:\Users\alice`, username: "alice", host: "devbox",
			in:   `C:\\Users\\alice\\bin`,
			want: `~\\bin`,
		},
		{
			name: "Linux のホーム",
			home: "/home/alice", username: "alice", host: "devbox",
			in:   "installed to /home/alice/.cargo/bin",
			want: "installed to ~/.cargo/bin",
		},
		{
			name: "ホーム外に現れるユーザー名",
			home: "/home/alice", username: "alice", host: "devbox",
			in:   "owner: alice (uid 1000)",
			want: "owner: <user> (uid 1000)",
		},
		{
			name: "単語の一部は置き換えない",
			home: "/home/alice", username: "alice", host: "devbox",
			in:   "malice and alicein",
			want: "malice and alicein",
		},
		{
			name: "ホスト名",
			home: "/home/alice", username: "alice", host: "devbox",
			in:   "host devbox responded",
			want: "host <host> responded",
		},
		{
			name: "短いユーザー名・ホスト名は一般の語を壊さないため対象外",
			home: "/h", username: "ab", host: "x",
			in:   "ab x /h/file",
			want: "ab x /h/file",
		},
		{
			name: "空の情報は何も置き換えない",
			home: "", username: "", host: "",
			in:   "plain text",
			want: "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRedactorFor(tt.home, tt.username, tt.host)
			assert.Equal(t, tt.want, r.redact(tt.in))
		})
	}
}

func TestHomeVariants(t *testing.T) {
	tests := []struct {
		name string
		home string
		want []string
	}{
		{"空", "", nil},
		{"短すぎる", "/h", nil},
		{"末尾の区切りは除く", `C:\Users\alice\`, []string{`C:\\Users\\alice`, `C:\Users\alice`, `C:/Users/alice`}},
		{"POSIX", "/home/alice", []string{`\home\alice`, "/home/alice"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatch(t, tt.want, homeVariants(tt.home))
		})
	}
}
