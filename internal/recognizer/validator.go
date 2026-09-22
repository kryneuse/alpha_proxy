// This file implements real checksum algorithms where they exist
// (INN control digits, Luhn for card numbers) and format validators for
// documents that have no reliable checksum.
package recognizer

import "strconv"

// Inn10 validates a 10-digit Russian INN using its control digit.
func Inn10(s string) bool {
	if len(s) != 10 {
		return false
	}
	weights := []int{2, 4, 10, 3, 5, 9, 4, 6, 8}
	sum := 0
	for i := 0; i < 9; i++ {
		d, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return false
		}
		sum += d * weights[i]
	}
	control := sum % 11 % 10
	last, err := strconv.Atoi(string(s[9]))
	if err != nil {
		return false
	}
	return control == last
}

// Inn12 validates a 12-digit Russian INN using its two control digits.
func Inn12(s string) bool {
	if len(s) != 12 {
		return false
	}
	weights1 := []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}
	weights2 := []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8}

	sum1 := 0
	for i := 0; i < 10; i++ {
		d, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return false
		}
		sum1 += d * weights1[i]
	}
	control1 := sum1 % 11 % 10

	sum2 := 0
	for i := 0; i < 11; i++ {
		d, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return false
		}
		sum2 += d * weights2[i]
	}
	control2 := sum2 % 11 % 10

	d10, _ := strconv.Atoi(string(s[10]))
	d11, _ := strconv.Atoi(string(s[11]))
	return control1 == d10 && control2 == d11
}

// Luhn validates a card number using the Luhn algorithm.
func Luhn(s string) bool {
	if len(s) < 2 {
		return false
	}
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		d, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return false
		}
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
