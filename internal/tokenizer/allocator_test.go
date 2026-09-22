package tokenizer

import (
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func TestSameValueSameToken(t *testing.T) {
	a := NewAllocator("")
	t1 := a.Allocate(pii.PIIKindPhone, "79123456789")
	t2 := a.Allocate(pii.PIIKindPhone, "79123456789")
	if t1 != t2 {
		t.Fatalf("expected same token for same value, got %q and %q", t1, t2)
	}
}

func TestDifferentValuesDifferentTokens(t *testing.T) {
	a := NewAllocator("")
	t1 := a.Allocate(pii.PIIKindPhone, "79123456789")
	t2 := a.Allocate(pii.PIIKindPhone, "79234567890")
	if t1 == t2 {
		t.Fatalf("expected different tokens for different values, got %q", t1)
	}
}

func TestSameValueDifferentKindsNotMerged(t *testing.T) {
	a := NewAllocator("")
	t1 := a.Allocate(pii.PIIKindPhone, "79123456789")
	t2 := a.Allocate(pii.PIIKindEmail, "79123456789")
	if t1 == t2 {
		t.Fatalf("expected different tokens for same value of different kinds, got %q", t1)
	}
}

func TestSeparateNumberingPerKind(t *testing.T) {
	a := NewAllocator("")
	phone1 := a.Allocate(pii.PIIKindPhone, "79123456789")
	email1 := a.Allocate(pii.PIIKindEmail, "a@example.com")
	phone2 := a.Allocate(pii.PIIKindPhone, "79234567890")
	email2 := a.Allocate(pii.PIIKindEmail, "b@example.com")

	if phone1 != "<PHONE_1>" {
		t.Fatalf("expected <PHONE_1>, got %q", phone1)
	}
	if phone2 != "<PHONE_2>" {
		t.Fatalf("expected <PHONE_2>, got %q", phone2)
	}
	if email1 != "<EMAIL_1>" {
		t.Fatalf("expected <EMAIL_1>, got %q", email1)
	}
	if email2 != "<EMAIL_2>" {
		t.Fatalf("expected <EMAIL_2>, got %q", email2)
	}
}

func TestSkipsTokenAlreadyInText(t *testing.T) {
	a := NewAllocator("contact <PHONE_1> and <PHONE_2>")
	t1 := a.Allocate(pii.PIIKindPhone, "79123456789")
	if t1 != "<PHONE_3>" {
		t.Fatalf("expected <PHONE_3> (skipping existing 1 and 2), got %q", t1)
	}
}

func TestIndependentAllocators(t *testing.T) {
	a1 := NewAllocator("")
	a2 := NewAllocator("")

	t1 := a1.Allocate(pii.PIIKindPhone, "79123456789")
	t2 := a2.Allocate(pii.PIIKindPhone, "79123456789")

	if t1 != "<PHONE_1>" {
		t.Fatalf("expected first allocator <PHONE_1>, got %q", t1)
	}
	if t2 != "<PHONE_1>" {
		t.Fatalf("expected second allocator <PHONE_1>, got %q", t2)
	}
}

func TestTokenFormat(t *testing.T) {
	a := NewAllocator("")
	phone := a.Allocate(pii.PIIKindPhone, "79123456789")
	email := a.Allocate(pii.PIIKindEmail, "a@example.com")
	fullName := a.Allocate(pii.PIIKindFullName, "Иванов Иван Иванович")

	if phone != "<PHONE_1>" {
		t.Fatalf("expected <PHONE_1>, got %q", phone)
	}
	if email != "<EMAIL_1>" {
		t.Fatalf("expected <EMAIL_1>, got %q", email)
	}
	if fullName != "<FULL_NAME_1>" {
		t.Fatalf("expected <FULL_NAME_1>, got %q", fullName)
	}
}
