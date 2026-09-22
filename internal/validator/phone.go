package validator

func NormalizePhone(number string) (string, bool) {
	var digits []byte
	hasPlus := false
	for i, r := range number {
		switch {
		case r >= '0' && r <= '9':
			digits = append(digits, byte(r))
		case r == ' ' || r == '-' || r == '(' || r == ')':
			// allowed separators, skipped
		case r == '+':
			if i != 0 || hasPlus {
				return "", false
			}
			hasPlus = true
		default:
			return "", false
		}
	}

	var normalized string
	switch len(digits) {
	case 10:
		if digits[0] != '9' || hasPlus {
			return "", false
		}
		normalized = "+7" + string(digits)
	case 11:
		switch digits[0] {
		case '7':
			normalized = "+7" + string(digits[1:])
		case '8':
			if hasPlus {
				return "", false
			}
			normalized = "+7" + string(digits[1:])
		default:
			return "", false
		}
	default:
		return "", false
	}

	return normalized, true
}
