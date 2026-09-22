package validator

func IsValidINN(inn string) bool {
	if len(inn) != 10 && len(inn) != 12 {
		return false
	}

	digits := make([]int, len(inn))
	for i, r := range inn {
		if r < '0' || r > '9' {
			return false
		}
		digits[i] = int(r - '0')
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

	switch len(inn) {
	case 10:
		return inn10CheckDigit(digits) == digits[9]
	case 12:
		return inn12CheckDigit1(digits) == digits[10] && inn12CheckDigit2(digits) == digits[11]
	default:
		return false
	}
}

func inn10CheckDigit(digits []int) int {
	weights := []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
	return checksum(digits[:9], weights) % 11 % 10
}

func inn12CheckDigit1(digits []int) int {
	weights := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	return checksum(digits[:10], weights) % 11 % 10
}

func inn12CheckDigit2(digits []int) int {
	weights := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	return checksum(digits[:11], weights) % 11 % 10
}

func checksum(digits []int, weights []int) int {
	sum := 0
	for i, d := range digits {
		sum += d * weights[i]
	}
	return sum
}
