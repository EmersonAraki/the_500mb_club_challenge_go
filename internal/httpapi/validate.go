package httpapi

// validID reports whether id matches ^[a-zA-Z0-9_-]{1,64}$. Implemented by hand
// to avoid a regexp allocation on the hot path.
func validID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
