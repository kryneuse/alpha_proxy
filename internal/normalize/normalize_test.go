package normalize

import "testing"

// TestByteToRuneCyrillic asserts that ByteToRune maps byte offsets to rune
// indices correctly for multi-byte Cyrillic text.
func TestByteToRuneCyrillic(t *testing.T) {
	// "Иван Петров" — each Cyrillic letter is 2 bytes.
	txt := New("Иван Петров")
	// Normalized: "иван петров"
	// Byte offsets: и(0-2) в(2-4) а(4-6) н(6-8) ' '(8-9) п(9-11) е(11-13) т(13-15) р(15-17) о(17-19) в(19-21)
	if got := txt.ByteToRune(0); got != 0 {
		t.Errorf("ByteToRune(0) = %d, want 0", got)
	}
	if got := txt.ByteToRune(2); got != 1 {
		t.Errorf("ByteToRune(2) = %d, want 1", got)
	}
	if got := txt.ByteToRune(8); got != 4 {
		t.Errorf("ByteToRune(8) = %d, want 4", got)
	}
	if got := txt.ByteToRune(9); got != 5 {
		t.Errorf("ByteToRune(9) = %d, want 5", got)
	}
	if got := txt.ByteToRune(21); got != 11 {
		t.Errorf("ByteToRune(21) = %d, want 11", got)
	}
}

// TestRuneToByteCyrillic asserts that RuneToByte maps rune indices to byte
// offsets correctly for multi-byte Cyrillic text.
func TestRuneToByteCyrillic(t *testing.T) {
	txt := New("Иван Петров")
	if got := txt.RuneToByte(0); got != 0 {
		t.Errorf("RuneToByte(0) = %d, want 0", got)
	}
	if got := txt.RuneToByte(1); got != 2 {
		t.Errorf("RuneToByte(1) = %d, want 2", got)
	}
	if got := txt.RuneToByte(4); got != 8 {
		t.Errorf("RuneToByte(4) = %d, want 8", got)
	}
	if got := txt.RuneToByte(5); got != 9 {
		t.Errorf("RuneToByte(5) = %d, want 9", got)
	}
	if got := txt.RuneToByte(11); got != 21 {
		t.Errorf("RuneToByte(11) = %d, want 21", got)
	}
}

// TestMapSpanRoundTrip asserts that MapSpan(ByteToRune(start), ByteToRune(end))
// recovers the original byte span.
func TestMapSpanRoundTrip(t *testing.T) {
	txt := New("Клиент: Иванов Иван Петрович")
	// Find "Иванов" in the original.
	orig := "Иванов"
	start := indexOf(txt.Original, orig)
	end := start + len(orig)
	ns := txt.ByteToRune(start)
	ne := txt.ByteToRune(end)
	os, oe := txt.MapSpan(ns, ne)
	if os != start || oe != end {
		t.Errorf("round trip: got [%d,%d), want [%d,%d)", os, oe, start, end)
	}
	if txt.Original[os:oe] != orig {
		t.Errorf("round trip text: got %q, want %q", txt.Original[os:oe], orig)
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
