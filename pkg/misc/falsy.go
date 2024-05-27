package misc

import "strings"

// Falsy returns true if v is falsy.. and yes, I realize how that might be confusing
func Falsy(v any) bool {
	switch v := v.(type) {
	case bool:
		return !v
	case int:
		return v != 1
	case string:
		if v == "" {
			return true
		}
		v = strings.ToLower(v)
		v = strings.TrimSpace(v)
		switch v {
		case "0":
			fallthrough
		case "false":
			return true
		}
	default:
		if v == nil {
			return true
		}
	}
	return false
}
