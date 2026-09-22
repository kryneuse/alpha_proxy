package tokenizer

import (
	"context"
	"errors"
	"testing"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

func TestDetokenizeBasic(t *testing.T) {
	text := "call <PHONE_1> now"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}

	got, err := Detokenize(context.Background(), text, mappings)
	if err != nil {
		t.Fatalf("Detokenize returned error: %v", err)
	}
	if got != "call 79123456789 now" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestDetokenizeRepeatedToken(t *testing.T) {
	text := "<PHONE_1> and <PHONE_1>"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 0, End: 11},
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 16, End: 27},
	}

	got, err := Detokenize(context.Background(), text, mappings)
	if err != nil {
		t.Fatalf("Detokenize returned error: %v", err)
	}
	if got != "79123456789 and 79123456789" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestDetokenizePhone1AndPhone10(t *testing.T) {
	text := "<PHONE_1> and <PHONE_10>"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 0, End: 11},
		{Token: "<PHONE_10>", Original: "79234567890", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 16, End: 27},
	}

	got, err := Detokenize(context.Background(), text, mappings)
	if err != nil {
		t.Fatalf("Detokenize returned error: %v", err)
	}
	if got != "79123456789 and 79234567890" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestDetokenizeUnknownToken(t *testing.T) {
	text := "call <EMAIL_1> now"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}

	_, err := Detokenize(context.Background(), text, mappings)
	if !errors.Is(err, pii.ErrUnknownToken) {
		t.Fatalf("expected ErrUnknownToken, got %v", err)
	}
}

func TestDetokenizeConflictingMappings(t *testing.T) {
	text := "call <PHONE_1> now"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
		{Token: "<PHONE_1>", Original: "79234567890", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}

	_, err := Detokenize(context.Background(), text, mappings)
	if !errors.Is(err, pii.ErrUnknownToken) {
		t.Fatalf("expected ErrUnknownToken for conflict, got %v", err)
	}
}

func TestDetokenizeOrdinaryAngleBrackets(t *testing.T) {
	text := "see <hello world> and <PHONE_1>"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 20, End: 31},
	}

	got, err := Detokenize(context.Background(), text, mappings)
	if err != nil {
		t.Fatalf("Detokenize returned error: %v", err)
	}
	if got != "see <hello world> and 79123456789" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestDetokenizeCyrillic(t *testing.T) {
	text := "звони <PHONE_1> пожалуйста"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 10, End: 21},
	}

	got, err := Detokenize(context.Background(), text, mappings)
	if err != nil {
		t.Fatalf("Detokenize returned error: %v", err)
	}
	if got != "звони 79123456789 пожалуйста" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestDetokenizeEmptyMappings(t *testing.T) {
	text := "plain text without tokens"

	got, err := Detokenize(context.Background(), text, nil)
	if err != nil {
		t.Fatalf("Detokenize returned error: %v", err)
	}
	if got != text {
		t.Fatalf("expected unchanged text, got %q", got)
	}
}

func TestDetokenizeCancelledContext(t *testing.T) {
	text := "call <PHONE_1> now"
	mappings := []pii.TokenMapping{
		{Token: "<PHONE_1>", Original: "79123456789", Kind: pii.PIIKindPhone, Source: pii.SourceReg, Start: 5, End: 16},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Detokenize(ctx, text, mappings)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
