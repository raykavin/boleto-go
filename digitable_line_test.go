package boleto

import (
	"errors"
	"testing"
	"time"
)

// TestDigitableLine_BankSlip_RoundTrip renders a built barcode as its linha
// digitável and parses it back. Because digitableLineToBarcode47 verifies
// all three field check digits, a renderer that computed any of them wrongly
// could not survive this trip.
func TestDigitableLine_BankSlip_RoundTrip(t *testing.T) {
	due := time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)
	barcode, err := BuildBankSlipBarcode("237", &due, 15075, "0000000012345678901234567")
	if err != nil {
		t.Fatalf("BuildBankSlipBarcode error = %v", err)
	}

	line, err := DigitableLine(barcode)
	if err != nil {
		t.Fatalf("DigitableLine error = %v", err)
	}
	if len(line) != 47 {
		t.Fatalf("rendered a %d-digit line, want 47", len(line))
	}

	b, err := Parse(line)
	if err != nil {
		t.Fatalf("Parse(line) error = %v", err)
	}
	if b.Barcode != barcode {
		t.Errorf("recovered barcode = %s, want %s", b.Barcode, barcode)
	}
	if b.BankCode != "237" || b.AmountCents != 15075 {
		t.Errorf("fields = %s/%d, want 237/15075", b.BankCode, b.AmountCents)
	}
	if b.DueDate == nil || !b.DueDate.Equal(due) {
		t.Errorf("DueDate = %v, want %v", b.DueDate, due)
	}
}

// TestDigitableLine_Collection_RoundTrip does the same for both supported
// collection check digit rules.
func TestDigitableLine_Collection_RoundTrip(t *testing.T) {
	for _, valueType := range []byte{'6', '8'} {
		t.Run(string(valueType), func(t *testing.T) {
			barcode, err := BuildUtilityBillBarcode('3', valueType, 4599, "12345678901234567890123456789")
			if err != nil {
				t.Fatalf("BuildUtilityBillBarcode error = %v", err)
			}

			line, err := DigitableLine(barcode)
			if err != nil {
				t.Fatalf("DigitableLine error = %v", err)
			}
			if len(line) != 48 {
				t.Fatalf("rendered a %d-digit line, want 48", len(line))
			}

			b, err := Parse(line)
			if err != nil {
				t.Fatalf("Parse(line) error = %v", err)
			}
			if b.Barcode != barcode {
				t.Errorf("recovered barcode = %s, want %s", b.Barcode, barcode)
			}
		})
	}
}

// TestDigitableLine_RealGPSDocument is the strongest available check on the
// collection renderer: the fixture is a real GPS guide whose printed linha
// digitável is known, so the renderer has to reproduce it digit for digit
// rather than merely agree with this package's own decoder.
func TestDigitableLine_RealGPSDocument(t *testing.T) {
	line, err := DigitableLine(gpsBarcode)
	if err != nil {
		t.Fatalf("DigitableLine(gpsBarcode) error = %v", err)
	}
	if line != gpsDigitableLine {
		t.Errorf("DigitableLine(gpsBarcode) = %s, want %s", line, gpsDigitableLine)
	}
}

// TestDigitableLine_AcceptsEitherForm keeps DigitableLine idempotent: handed
// a line it already produced, or a line with the separators a document is
// printed with, it must return the same digits.
func TestDigitableLine_AcceptsEitherForm(t *testing.T) {
	for _, input := range []string{
		gpsBarcode,
		gpsDigitableLine,
		"8580-0000.1230 61603852626107162624738599731022",
	} {
		line, err := DigitableLine(input)
		if err != nil {
			t.Fatalf("DigitableLine(%q) error = %v", input, err)
		}
		if line != gpsDigitableLine {
			t.Errorf("DigitableLine(%q) = %s, want %s", input, line, gpsDigitableLine)
		}
	}
}

// TestDigitableLine_RejectsInvalidInput confirms DigitableLine refuses to
// render anything Parse would not accept: it must not launder a corrupt
// barcode into a well-formed-looking line.
func TestDigitableLine_RejectsInvalidInput(t *testing.T) {
	tampered := []byte(gpsBarcode)
	tampered[3] = '0' + (tampered[3]-'0'+1)%10

	cases := map[string]struct {
		input string
		want  error
	}{
		"tampered general digit": {string(tampered), ErrInvalidGeneralCheckDigit},
		"too short":              {"12345", ErrInvalidLength},
		"empty":                  {"", ErrEmptyInput},
		"non-numeric":            {"ABCD567890123456789012345678901234567890123X", ErrNonNumeric},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DigitableLine(tc.input); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestBoletoDigitableLine_Method covers the method form, including the
// KindBankSlip path that DigitableLine reaches only after a successful
// Parse.
func TestBoletoDigitableLine_Method(t *testing.T) {
	b, err := Parse(gpsDigitableLine)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	line, err := b.DigitableLine()
	if err != nil {
		t.Fatalf("(*Boleto).DigitableLine error = %v", err)
	}
	if line != gpsDigitableLine {
		t.Errorf("line = %s, want %s", line, gpsDigitableLine)
	}

	// A zero-value Boleto carries no barcode and must be refused rather than
	// panicking on a slice out of range.
	if _, err := (&Boleto{}).DigitableLine(); !errors.Is(err, ErrInvalidLength) {
		t.Errorf("zero Boleto error = %v, want ErrInvalidLength", err)
	}
}
