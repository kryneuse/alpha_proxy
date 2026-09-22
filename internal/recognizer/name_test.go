package recognizer

import (
	"testing"
)

// TestTokenizerByteOffsets asserts that tokenize produces correct byte offsets
// for Cyrillic and mixed Cyrillic/Latin text.
func TestTokenizerByteOffsets(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"иван петров", []string{"иван", "петров"}},
		{"сотрудник иван петров подал заявку", []string{"сотрудник", "иван", "петров", "подал", "заявку"}},
		{"иван петров гулял в парке", []string{"иван", "петров", "гулял", "в", "парке"}},
		{"hello world", []string{"hello", "world"}},
		{"иван petrov 123", []string{"иван", "petrov"}},
		{"смешанный текст with latin", []string{"смешанный", "текст", "with", "latin"}},
	}
	for _, c := range cases {
		got := tokenize(c.text)
		if len(got) != len(c.want) {
			t.Errorf("tokenize(%q): expected %d tokens, got %d (%+v)", c.text, len(c.want), len(got), got)
			continue
		}
		for i, w := range c.want {
			if got[i].text != w {
				t.Errorf("tokenize(%q)[%d]: expected %q, got %q", c.text, i, w, got[i].text)
			}
			// Verify byte offsets slice the original string correctly.
			if c.text[got[i].start:got[i].end] != got[i].text {
				t.Errorf("tokenize(%q)[%d]: offset mismatch %q != %q", c.text, i, c.text[got[i].start:got[i].end], got[i].text)
			}
		}
	}
}

// TestTokenizerByteOffsetsCyrillic asserts byte offsets are correct for
// multi-byte Cyrillic characters (each Cyrillic letter is 2 bytes).
func TestTokenizerByteOffsetsCyrillic(t *testing.T) {
	// "иван" = и(2)в(2)а(2)н(2) = 8 bytes.
	got := tokenize("иван")
	if len(got) != 1 {
		t.Fatalf("expected 1 token, got %d", len(got))
	}
	if got[0].start != 0 || got[0].end != 8 {
		t.Errorf("expected byte offsets [0,8), got [%d,%d)", got[0].start, got[0].end)
	}
	if got[0].text != "иван" {
		t.Errorf("expected text 'иван', got %q", got[0].text)
	}

	// "иван петров" — иван is 8 bytes, space is 1, петров is 12 bytes.
	got = tokenize("иван петров")
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(got))
	}
	if got[0].start != 0 || got[0].end != 8 {
		t.Errorf("token 0: expected [0,8), got [%d,%d)", got[0].start, got[0].end)
	}
	if got[1].start != 9 || got[1].end != 21 {
		t.Errorf("token 1: expected [9,21), got [%d,%d)", got[1].start, got[1].end)
	}
}

// TestTokenizerByteOffsetsMixed asserts byte offsets are correct for mixed
// Cyrillic and Latin text.
func TestTokenizerByteOffsetsMixed(t *testing.T) {
	// "иван petrov" — иван is 8 bytes, space 1, petrov is 6 bytes.
	got := tokenize("иван petrov")
	if len(got) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(got))
	}
	if got[0].start != 0 || got[0].end != 8 {
		t.Errorf("token 0: expected [0,8), got [%d,%d)", got[0].start, got[0].end)
	}
	if got[1].start != 9 || got[1].end != 15 {
		t.Errorf("token 1: expected [9,15), got [%d,%d)", got[1].start, got[1].end)
	}
}
