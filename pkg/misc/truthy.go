package misc

import "strings"

func Truthy(v any) bool {
	switch v := v.(type) {
	case bool:
		return v
	case int:
		return v == 1
	case string:
		if v == "" {
			return false
		}
		v = strings.ToLower(v)
		v = strings.TrimSpace(v)
		switch v {
		case "1":
			fallthrough
		case "true":
			return true
		}
	default:
		if v != nil {
			return true
		}
	}
	return false
}
