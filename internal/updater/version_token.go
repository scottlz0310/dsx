package updater

// looksLikeVersionToken はバージョンらしいトークンかを判定します。
func looksLikeVersionToken(token string) bool {
	if token == "" {
		return false
	}

	hasDigit := false
	for _, r := range token {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '.', r == '-', r == '_', r == '+':
		default:
			return false
		}
	}

	return hasDigit
}
