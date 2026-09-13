package boleto

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestBuildBankSlipBarcode_RoundTrip(t *testing.T) {
	due := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	barcode, err := BuildBankSlipBarcode("237", &due, 987654, "0000000000000000000000001")
	if err == nil && len(barcode) != 44 {
		t.Fatalf("expected a 44-digit barcode, got %d digits", len(barcode))
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := Parse(barcode)
	if err != nil {
		t.Fatalf("unexpected error parsing a freshly built barcode: %v", err)
	}
	if b.AmountCents != 987654 {
		t.Errorf("expected amount 987654, got %d", b.AmountCents)
	}
	if b.DueDate == nil || !b.DueDate.Equal(due) {
		t.Errorf("expected due date %v, got %v", due, b.DueDate)
	}
	if b.BankCode != "237" {
		t.Errorf("expected bank code 237, got %s", b.BankCode)
	}
}

func TestBuildBankSlipBarcode_NilDueDate(t *testing.T) {
	barcode, err := BuildBankSlipBarcode("001", nil, 100, "0000000000000000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := Parse(barcode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.DueDate != nil {
		t.Errorf("expected nil due date, got %v", b.DueDate)
	}
}

func TestBuildBankSlipBarcode_InvalidInputs(t *testing.T) {
	if _, err := BuildBankSlipBarcode("12", nil, 100, "0000000000000000000000000"); err != ErrInvalidBankCode {
		t.Errorf("expected ErrInvalidBankCode, got %v", err)
	}
	if _, err := BuildBankSlipBarcode("001", nil, 100, "123"); err != ErrInvalidFreeField {
		t.Errorf("expected ErrInvalidFreeField, got %v", err)
	}
}

func TestBuildBankSlipBarcode_DueDateOutOfRange(t *testing.T) {
	// Beyond dueDateNewEpoch + (dueDateFactorMax-dueDateFactorNewBase) days
	// (2025-02-22 + 8999 days, ~2049-08), no factor under the new epoch can
	// represent the date anymore.
	tooFar := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := BuildBankSlipBarcode("001", &tooFar, 100, "0000000000000000000000000"); err != ErrDueDateOutOfRange {
		t.Errorf("expected ErrDueDateOutOfRange, got %v", err)
	}

	// A date that predates dueDateNewEpoch but would need a factor FEBRABAN
	// reassigned to the new epoch (>= dueDateFactorNewBase under the old
	// base) is now ambiguous and must also be rejected, even though the
	// 4-digit field could technically still hold that numeric factor.
	reassignedRange := time.Date(2015, time.March, 15, 0, 0, 0, 0, time.UTC)
	if _, err := BuildBankSlipBarcode("001", &reassignedRange, 100, "0000000000000000000000000"); err != ErrDueDateOutOfRange {
		t.Errorf("expected ErrDueDateOutOfRange for a pre-cutover date in the reassigned factor range, got %v", err)
	}
}

// TestBuildBankSlipBarcode_EpochBoundaries is the achado 12 regression
// test: it exercises the exact days around FEBRABAN's 2025-02-22 "fator de
// vencimento" base-date reset, plus a legacy pre-reset date still within
// the original epoch's unambiguous range, proving the encoder picks the
// correct base in each case and that Parse decodes the identical date back
// out (the reason encode/decode share decodeDueDateFactor/
// encodeDueDateFactor instead of duplicating the epoch logic).
func TestBuildBankSlipBarcode_EpochBoundaries(t *testing.T) {
	cases := []struct {
		name string
		due  time.Time
	}{
		{"legacy date within the unambiguous old-epoch range", time.Date(1999, time.January, 10, 0, 0, 0, 0, time.UTC)},
		{"last day representable under the old epoch alone", dueDateEpochPlusDays(t, dueDateFactorNewBase-1)},
		{"the new epoch's first day (day the reset takes effect)", time.Date(2025, time.February, 22, 0, 0, 0, 0, time.UTC)},
		{"day after the new epoch's first day", time.Date(2025, time.February, 23, 0, 0, 0, 0, time.UTC)},
		{"a typical near-future due date (this package's real-world case)", time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC)},
		{"last day representable under the new epoch", dueDateNewEpochPlusDays(t, dueDateFactorMax-dueDateFactorNewBase)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			barcode, err := BuildBankSlipBarcode("341", &tc.due, 100000, "0000000000000000000000001")
			if err != nil {
				t.Fatalf("unexpected error building barcode for %v: %v", tc.due, err)
			}
			b, err := Parse(barcode)
			if err != nil {
				t.Fatalf("unexpected error parsing barcode for %v: %v", tc.due, err)
			}
			if b.DueDate == nil || !b.DueDate.Equal(tc.due) {
				t.Errorf("expected due date %v, got %v", tc.due, b.DueDate)
			}
		})
	}
}

func dueDateEpochPlusDays(t *testing.T, days int) time.Time {
	t.Helper()
	return dueDateEpoch.AddDate(0, 0, days)
}

func dueDateNewEpochPlusDays(t *testing.T, days int) time.Time {
	t.Helper()
	return dueDateNewEpoch.AddDate(0, 0, days)
}

// TestDecodeDueDateFactor_KnownValues locks down the two known, previously
// wrong data points from the achado 12 audit finding: a factor computed
// under the pre-2025 assumption for a September/2026 due date used to
// decode to 1999-04-30 instead. Both now decode under the new epoch.
func TestDecodeDueDateFactor_KnownValues(t *testing.T) {
	got := decodeDueDateFactor(dueDateFactorNewBase) // exactly the reset day
	want := time.Date(2025, time.February, 22, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("decodeDueDateFactor(%d) = %v, want %v", dueDateFactorNewBase, got, want)
	}

	// The reported audit scenario: a real bank-issued factor for a
	// September/2026 due date is dueDateFactorNewBase plus ~570 days past
	// the new base. The unpatched decoder ignored the reset and always
	// added the raw factor to the 1997 epoch, landing on 2002-01-24 for
	// this exact factor instead of the correct 2026 date.
	factor := dueDateFactorNewBase + 570
	got = decodeDueDateFactor(factor)
	want = time.Date(2026, time.September, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("decodeDueDateFactor(%d) = %v, want %v (must never resolve to the 1997 epoch for a post-reset factor)", factor, got, want)
	}
	wrongLegacyInterpretation := dueDateEpoch.AddDate(0, 0, factor)
	if got.Equal(wrongLegacyInterpretation) {
		t.Errorf("decodeDueDateFactor(%d) incorrectly matches the old, unpatched 1997-epoch interpretation %v", factor, wrongLegacyInterpretation)
	}
}

// TestBuildBankSlipBarcode_AmountOutOfRange covers the guard that replaced a
// silent corruption: a negative amount used to emit a '-' into the barcode,
// and an amount wider than the 10-digit value field used to emit a 45-digit
// string. Both produced something that was not a barcode, and neither was
// reported.
func TestBuildBankSlipBarcode_AmountOutOfRange(t *testing.T) {
	const freeField = "0000000012345678901234567"
	for name, amount := range map[string]int64{
		"negative":      -5,
		"negative one":  -1,
		"one over max":  maxBankSlipAmountCents + 1,
		"eleven digits": 99999999999,
		"max int64":     math.MaxInt64,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildBankSlipBarcode("237", nil, amount, freeField); !errors.Is(err, ErrAmountOutOfRange) {
				t.Errorf("BuildBankSlipBarcode(amount=%d) error = %v, want ErrAmountOutOfRange", amount, err)
			}
		})
	}
}

// TestBuildBankSlipBarcode_AmountBoundaries proves the guard rejects only
// what overflows the field: zero and the widest value it holds must still
// build, and must survive a round trip.
func TestBuildBankSlipBarcode_AmountBoundaries(t *testing.T) {
	const freeField = "0000000012345678901234567"
	for _, amount := range []int64{0, 1, maxBankSlipAmountCents} {
		barcode, err := BuildBankSlipBarcode("237", nil, amount, freeField)
		if err != nil {
			t.Fatalf("BuildBankSlipBarcode(amount=%d) error = %v", amount, err)
		}
		if len(barcode) != 44 {
			t.Errorf("amount %d produced a %d-digit barcode, want 44", amount, len(barcode))
		}
		b, err := Parse(barcode)
		if err != nil {
			t.Fatalf("Parse of a freshly built barcode error = %v", err)
		}
		if b.AmountCents != amount {
			t.Errorf("AmountCents = %d, want %d", b.AmountCents, amount)
		}
	}
}

// TestBuildUtilityBillBarcode_RoundTrip exercises the collection builder
// through both supported check digit rules, in both the barcode and linha
// digitável forms.
func TestBuildUtilityBillBarcode_RoundTrip(t *testing.T) {
	const freeField = "12345678901234567890123456789"
	for _, valueType := range []byte{'6', '8'} {
		t.Run(string(valueType), func(t *testing.T) {
			barcode, err := BuildUtilityBillBarcode('3', valueType, 4599, freeField)
			if err != nil {
				t.Fatalf("BuildUtilityBillBarcode error = %v", err)
			}
			if len(barcode) != 44 {
				t.Fatalf("built a %d-digit barcode, want 44", len(barcode))
			}
			if barcode[0] != '8' || barcode[1] != '3' || barcode[2] != valueType {
				t.Errorf("head = %q, want 83%c", barcode[0:3], valueType)
			}

			b, err := Parse(barcode)
			if err != nil {
				t.Fatalf("Parse of a freshly built collection barcode error = %v", err)
			}
			if b.Kind != KindUtilityBill {
				t.Errorf("Kind = %v, want KindUtilityBill", b.Kind)
			}
			if b.AmountCents != 4599 {
				t.Errorf("AmountCents = %d, want 4599", b.AmountCents)
			}
			if b.DueDate != nil {
				t.Errorf("DueDate = %v, want nil", b.DueDate)
			}
			if b.BankCode != "" {
				t.Errorf("BankCode = %q, want empty", b.BankCode)
			}
			if b.FreeField != freeField {
				t.Errorf("FreeField = %q, want %q", b.FreeField, freeField)
			}

			line := mustCollectionDigitableLine(t, barcode)
			fromLine, err := Parse(line)
			if err != nil {
				t.Fatalf("Parse(linha digitável) error = %v", err)
			}
			if fromLine.Barcode != barcode {
				t.Errorf("recovered barcode = %s, want %s", fromLine.Barcode, barcode)
			}
		})
	}
}

func TestBuildUtilityBillBarcode_InvalidInputs(t *testing.T) {
	const freeField = "12345678901234567890123456789"
	cases := []struct {
		name      string
		segment   byte
		valueType byte
		amount    int64
		freeField string
		want      error
	}{
		{"segment zero", '0', '6', 100, freeField, ErrInvalidSegment},
		{"segment non-digit", 'A', '6', 100, freeField, ErrInvalidSegment},
		{"quantidade de moeda mod10", '3', '7', 100, freeField, ErrUnsupportedUtilityBillVariant},
		{"quantidade de moeda mod11", '3', '9', 100, freeField, ErrUnsupportedUtilityBillVariant},
		{"value type off table", '3', '0', 100, freeField, ErrUnsupportedUtilityBillVariant},
		{"free field too short", '3', '6', 100, "1234567890123456789012345", ErrInvalidFreeField},
		{"free field non-numeric", '3', '6', 100, "1234567890123456789012345678A", ErrNonNumeric},
		{"negative amount", '3', '6', -1, freeField, ErrAmountOutOfRange},
		{"amount over field", '3', '6', maxUtilityBillAmountCents + 1, freeField, ErrAmountOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildUtilityBillBarcode(tc.segment, tc.valueType, tc.amount, tc.freeField); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestBuildUtilityBillBarcode_AmountBoundaries mirrors the bank slip
// boundary test against the wider 11-digit collection value field.
func TestBuildUtilityBillBarcode_AmountBoundaries(t *testing.T) {
	const freeField = "12345678901234567890123456789"
	for _, amount := range []int64{0, maxUtilityBillAmountCents} {
		barcode, err := BuildUtilityBillBarcode('3', '8', amount, freeField)
		if err != nil {
			t.Fatalf("BuildUtilityBillBarcode(amount=%d) error = %v", amount, err)
		}
		b, err := Parse(barcode)
		if err != nil {
			t.Fatalf("Parse error = %v", err)
		}
		if b.AmountCents != amount {
			t.Errorf("AmountCents = %d, want %d", b.AmountCents, amount)
		}
	}
}

// TestBuildUtilityBillBarcode_AllSegments checks every segment the builder
// accepts actually produces a parseable document, rather than only the one
// the other tests happen to use.
func TestBuildUtilityBillBarcode_AllSegments(t *testing.T) {
	for segment := byte('1'); segment <= '9'; segment++ {
		barcode, err := BuildUtilityBillBarcode(segment, '6', 1000, "12345678901234567890123456789")
		if err != nil {
			t.Fatalf("segment %c: %v", segment, err)
		}
		if _, err := Parse(barcode); err != nil {
			t.Errorf("segment %c: Parse error = %v", segment, err)
		}
	}
}
