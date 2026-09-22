package validator

func IsValidLuhn(number string) bool {
	var digits []byte
	for _, r := range number {
		switch {
		case r >= '0' && r <= '9':
			digits = append(digits, byte(r))
		case r == ' ' || r == '-':
			// allowed separators, skipped
		default:
			return false
		}
	}

	if len(digits) < 13 || len(digits) > 19 {
		return false
	}

	allSame := true
	first := digits[0]
	for i := 1; i < len(digits); i++ {
		if digits[i] != first {
			allSame = false
			break
		}
	}
	if allSame {
		return false
	}

	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		digit := int(digits[i] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}

	return sum%10 == 0
}
