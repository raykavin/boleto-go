package boleto

import (
	"errors"
	"strconv"
	"testing"
)

// buildBankSlipBarcode assembles a structurally valid 44-digit bank slip
// barcode from its parts, computing the correct general check digit the
// same algorithm the parser itself uses, which makes this a round-trip
// consistency test (build -> parse -> compare) rather than a check against
// an external fixture. Tamper tests below flip individual digits to prove
// the parser actually rejects bad input, not just accepts anything.
func buildBankSlipBarcode(t *testing.T, bankCode string, dueDateFactor int, amountCents int64, freeField string) string {
	t.Helper()
	if len(bankCode) != 3 || len(freeField) != 25 {
		t.Fatalf("bad test input: bankCode=%q freeField=%q", bankCode, freeField)
	}
	factorStr := zeroPad(strconv.Itoa(dueDateFactor), 4)
	valueStr := zeroPad(strconv.FormatInt(amountCents, 10), 10)

	dv := computeGeneralCheckDigitMod11(bankCode + "9" + "?" + factorStr + valueStr + freeField)
	barcode := bankCode + "9" + string(dv) + factorStr + valueStr + freeField
	if len(barcode) != 44 {
		t.Fatalf("built barcode has wrong length: %d", len(barcode))
	}
	return barcode
}

func zeroPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

func TestMod10_KnownProperties(t *testing.T) {
	// mod10("0") => weight 2 on the only digit: 0*2=0, sum=0, remainder=0 => DV '0'.
	if got := mod10("0"); got != '0' {
		t.Errorf("mod10(0) = %c, want 0", got)
	}
	// A well-known mod10 check: "123456789" verified by hand:
	// digits (right to left): 9,8,7,6,5,4,3,2,1 with weights 2,1,2,1,2,1,2,1,2
	// products: 18->9, 8, 14->5, 6, 10->1, 4, 6, 2, 2 = sum 43, remainder 3, dv=7
	if got := mod10("123456789"); got != '7' {
		t.Errorf("mod10(123456789) = %c, want 7", got)
	}
}

func TestParse_RoundTrip_BankSlip_Barcode(t *testing.T) {
	freeField := "1234567890123456789012345"
	barcode := buildBankSlipBarcode(t, "748", 5000, 123456, freeField)

	b, err := Parse(barcode)
	if err != nil {
		t.Fatalf("unexpected error parsing a freshly built, valid barcode: %v", err)
	}
	if b.Kind != KindBankSlip {
		t.Errorf("expected KindBankSlip, got %v", b.Kind)
	}
	if b.BankCode != "748" {
		t.Errorf("expected bank code 748, got %s", b.BankCode)
	}
	if b.AmountCents != 123456 {
		t.Errorf("expected amount 123456, got %d", b.AmountCents)
	}
	wantDue := decodeDueDateFactor(5000)
	if b.DueDate == nil || !b.DueDate.Equal(wantDue) {
		t.Errorf("expected due date %v, got %v", wantDue, b.DueDate)
	}
}

func TestParse_RoundTrip_BankSlip_NoDueDate(t *testing.T) {
	barcode := buildBankSlipBarcode(t, "001", 0, 999, "0000000000000000000000000")
	b, err := Parse(barcode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.DueDate != nil {
		t.Errorf("expected nil due date for factor 0000, got %v", b.DueDate)
	}
}

func TestParse_TamperedGeneralCheckDigit_Rejected(t *testing.T) {
	barcode := buildBankSlipBarcode(t, "748", 5000, 123456, "1234567890123456789012345")
	tampered := []byte(barcode)
	original := tampered[4]
	tampered[4] = '0' + (original-'0'+1)%10 // flip the general DV to a different digit
	if _, err := Parse(string(tampered)); !errors.Is(err, ErrInvalidGeneralCheckDigit) {
		t.Errorf("expected ErrInvalidGeneralCheckDigit, got %v", err)
	}
}

func TestParse_TamperedAmount_Rejected(t *testing.T) {
	barcode := buildBankSlipBarcode(t, "748", 5000, 123456, "1234567890123456789012345")
	tampered := []byte(barcode)
	// Change one of the value digits without recomputing the DV the
	// general check digit must catch this, since it covers the whole
	// barcode including the value field.
	tampered[10] = '0' + (tampered[10]-'0'+1)%10
	if _, err := Parse(string(tampered)); !errors.Is(err, ErrInvalidGeneralCheckDigit) {
		t.Errorf("expected the tampered amount to be caught by the general check digit, got %v", err)
	}
}

func TestParse_InvalidLength(t *testing.T) {
	if _, err := Parse("12345"); !errors.Is(err, ErrInvalidLength) {
		t.Errorf("expected ErrInvalidLength, got %v", err)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	if _, err := Parse(""); !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
	if _, err := Parse("   "); !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput for whitespace-only input, got %v", err)
	}
}

func TestParse_NonNumeric(t *testing.T) {
	if _, err := Parse("ABCD567890123456789012345678901234567890123X"); err == nil {
		t.Error("expected an error for non-numeric input")
	}
}

// TestParse_RoundTrip_BankSlip_DigitableLine builds a valid barcode, derives
// its corresponding 47-digit linha digitável by hand (mirroring
// digitableLineToBarcode47's own field layout in reverse), and confirms
// Parse accepts it and recovers the identical barcode.
func TestParse_RoundTrip_BankSlip_DigitableLine(t *testing.T) {
	freeField := "1234567890123456789012345"
	barcode := buildBankSlipBarcode(t, "104", 6000, 50000, freeField)

	bankAndCurrency := barcode[0:4]
	generalDV := barcode[4:5]
	fatorEValor := barcode[5:19]
	ff := barcode[19:44]

	data1 := bankAndCurrency + ff[0:5]
	data2 := ff[5:15]
	data3 := ff[15:25]

	digitable := data1 + string(mod10(data1)) +
		data2 + string(mod10(data2)) +
		data3 + string(mod10(data3)) +
		generalDV +
		fatorEValor

	if len(digitable) != 47 {
		t.Fatalf("test built a %d-digit linha digitável, want 47", len(digitable))
	}

	b, err := Parse(digitable)
	if err != nil {
		t.Fatalf("unexpected error parsing a valid linha digitável: %v", err)
	}
	if b.Barcode != barcode {
		t.Errorf("expected recovered barcode %s, got %s", barcode, b.Barcode)
	}
}

func TestParse_DigitableLine_TamperedFieldCheckDigit_Rejected(t *testing.T) {
	freeField := "1234567890123456789012345"
	barcode := buildBankSlipBarcode(t, "104", 6000, 50000, freeField)

	bankAndCurrency := barcode[0:4]
	generalDV := barcode[4:5]
	fatorEValor := barcode[5:19]
	ff := barcode[19:44]
	data1 := bankAndCurrency + ff[0:5]
	data2 := ff[5:15]
	data3 := ff[15:25]

	badDV1 := (mod10(data1)-'0'+1)%10 + '0'
	digitable := data1 + string(badDV1) +
		data2 + string(mod10(data2)) +
		data3 + string(mod10(data3)) +
		generalDV +
		fatorEValor

	if _, err := Parse(digitable); !errors.Is(err, ErrInvalidFieldCheckDigit) {
		t.Errorf("expected ErrInvalidFieldCheckDigit, got %v", err)
	}
}

// buildUtilityBillBarcode assembles a structurally valid 44-digit
// collection barcode of the mod10 variant: product(1, always '8') +
// segment(1, arbitrary) + value-type(1, '6' = valor efetivo/mod10) +
// general check digit(1) + value(11) + free field(29).
func buildUtilityBillBarcode(t *testing.T, amountCents int64, freeField29 string) string {
	t.Helper()
	return buildCollectionBarcode(t, '6', amountCents, freeField29)
}

// buildCollectionBarcode assembles a structurally valid 44-digit collection
// barcode for a given value-type digit, computing the general check digit
// with whichever rule that digit selects. It is what lets the mod10 ('6')
// and mod11 ('8') variants be exercised through one shared shape.
func buildCollectionBarcode(t *testing.T, valueType byte, amountCents int64, freeField29 string) string {
	t.Helper()
	if len(freeField29) != 29 {
		t.Fatalf("bad test input: freeField29 must be 29 digits, got %d", len(freeField29))
	}
	checkDigit, ok := collectionCheckDigit(valueType)
	if !ok {
		t.Fatalf("bad test input: value type %q is not a supported variant", valueType)
	}
	valueStr := zeroPad(strconv.FormatInt(amountCents, 10), 11)
	head := "8" + "1" + string(valueType)
	dv := collectionGeneralCheckDigit(head+"?"+valueStr+freeField29, checkDigit)
	barcode := head + string(dv) + valueStr + freeField29
	if len(barcode) != 44 {
		t.Fatalf("built collection barcode has wrong length: %d", len(barcode))
	}
	return barcode
}

// collectionDigitableLine renders a collection barcode as its 48-digit
// linha digitável: four 11-digit slices, each followed by its own check
// digit under the rule the barcode's value-type digit selects.
func collectionDigitableLine(t *testing.T, barcode string) string {
	t.Helper()
	checkDigit, ok := collectionCheckDigit(barcode[2])
	if !ok {
		t.Fatalf("bad test input: value type %q is not a supported variant", barcode[2])
	}
	line := ""
	for _, f := range [4]string{barcode[0:11], barcode[11:22], barcode[22:33], barcode[33:44]} {
		line += f + string(checkDigit(f))
	}
	if len(line) != 48 {
		t.Fatalf("built a %d-digit linha digitável, want 48", len(line))
	}
	return line
}

func TestParse_RoundTrip_UtilityBill(t *testing.T) {
	barcode := buildUtilityBillBarcode(t, 4599, "12345678901234567890123456789")
	b, err := Parse(barcode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Kind != KindUtilityBill {
		t.Errorf("expected KindUtilityBill, got %v", b.Kind)
	}
	if b.AmountCents != 4599 {
		t.Errorf("expected amount 4599, got %d", b.AmountCents)
	}
	if b.DueDate != nil {
		t.Error("expected nil due date for a utility bill")
	}
}

func TestParse_UtilityBill_TamperedCheckDigit_Rejected(t *testing.T) {
	barcode := buildUtilityBillBarcode(t, 4599, "12345678901234567890123456789")
	tampered := []byte(barcode)
	tampered[3] = '0' + (tampered[3]-'0'+1)%10
	if _, err := Parse(string(tampered)); !errors.Is(err, ErrInvalidGeneralCheckDigit) {
		t.Errorf("expected ErrInvalidGeneralCheckDigit, got %v", err)
	}
}

// TestParse_Collection_UnsupportedValueTypes pins which value-type digits
// this package refuses. '7' and '9' are the "quantidade de moeda" variants:
// their value field counts units of a reference currency, not centavos, so
// decoding them as money would be wrong. Everything outside FEBRABAN's
// table is refused too.
//
// This test used to assert that '7' selected "the unimplemented mod11
// variant", which had the FEBRABAN table backwards: the módulo is chosen by
// 6/7 against 8/9. '7' is a módulo 10 document and '8' the one this
// package rejected as malformed is a módulo 11 one.
func TestParse_Collection_UnsupportedValueTypes(t *testing.T) {
	barcode := buildUtilityBillBarcode(t, 4599, "12345678901234567890123456789")
	for _, valueType := range []byte{'0', '1', '5', '7', '9'} {
		t.Run(string(valueType), func(t *testing.T) {
			tampered := []byte(barcode)
			tampered[2] = valueType
			if _, err := Parse(string(tampered)); !errors.Is(err, ErrUnsupportedUtilityBillVariant) {
				t.Errorf("expected ErrUnsupportedUtilityBillVariant, got %v", err)
			}
		})
	}
}

func TestParse_RoundTrip_UtilityBill_DigitableLine(t *testing.T) {
	barcode := buildUtilityBillBarcode(t, 1000, "10000000000000000000000000000")
	digitable := collectionDigitableLine(t, barcode)

	b, err := Parse(digitable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Barcode != barcode {
		t.Errorf("expected recovered barcode %s, got %s", barcode, b.Barcode)
	}
}

func TestParse_WithConventionalSeparators(t *testing.T) {
	barcode := buildBankSlipBarcode(t, "748", 5000, 123456, "1234567890123456789012345")
	spaced := barcode[0:5] + "." + barcode[5:10] + " " + barcode[10:]
	b, err := Parse(spaced)
	if err != nil {
		t.Fatalf("unexpected error parsing a barcode with separators: %v", err)
	}
	if b.Barcode != barcode {
		t.Errorf("expected %s, got %s", barcode, b.Barcode)
	}
}

// gpsDigitableLine is a real GPS (Guia da Previdência Social) linha
// digitável that this package rejected as having a bad field check digit.
// It is a módulo 11 collection document: value-type digit '8' at position
// 3. Keeping the actual document, rather than a synthesised one, is what
// makes this a regression test for the report it came from.
const (
	gpsDigitableLine = "858000001239061603852620610716262474385997310225"
	gpsBarcode       = "85800000123061603852626107162624738599731022"
	gpsAmountCents   = int64(1230616)
)

// TestParse_Collection_Mod11_RealGPSDocument is the regression test for the
// reported failure: a valid GPS guide came back as
// "dígito verificador de campo da linha digitável inválido", which made the
// expense ineligible. Both the linha digitável and the barcode it encodes
// must now parse, and must agree.
func TestParse_Collection_Mod11_RealGPSDocument(t *testing.T) {
	t.Run("linha digitável", func(t *testing.T) {
		b, err := Parse(gpsDigitableLine)
		if err != nil {
			t.Fatalf("Parse(linha digitável) error = %v, want success", err)
		}
		if b.Kind != KindUtilityBill {
			t.Errorf("Kind = %v, want KindUtilityBill", b.Kind)
		}
		if b.Barcode != gpsBarcode {
			t.Errorf("Barcode = %s, want %s", b.Barcode, gpsBarcode)
		}
		if b.AmountCents != gpsAmountCents {
			t.Errorf("AmountCents = %d, want %d", b.AmountCents, gpsAmountCents)
		}
		if b.DueDate != nil {
			t.Error("expected nil due date for a collection document")
		}
	})

	t.Run("código de barras", func(t *testing.T) {
		b, err := Parse(gpsBarcode)
		if err != nil {
			t.Fatalf("Parse(barcode) error = %v, want success", err)
		}
		if b.AmountCents != gpsAmountCents {
			t.Errorf("AmountCents = %d, want %d", b.AmountCents, gpsAmountCents)
		}
	})
}

// TestMod11Collection_RemainderZeroYieldsZero pins the one place the
// collection rule and the bank slip rule disagree, and the reason the bank
// slip's mod11 could not simply be reused. The GPS document above carries a
// field ("06160385262") whose weighted sum is an exact multiple of 11 and
// whose printed check digit is 0; mod11 would answer 1 for it.
func TestMod11Collection_RemainderZeroYieldsZero(t *testing.T) {
	const field = "06160385262"
	if got := mod11Collection(field); got != '0' {
		t.Errorf("mod11Collection(%q) = %q, want '0'", field, got)
	}
	if got := mod11(field); got != '1' {
		t.Errorf("mod11(%q) = %q, want '1' the two rules are supposed to differ here", field, got)
	}
}

// TestParse_Collection_Mod11_RoundTrip exercises the mod11 variant through
// the same shape the mod10 one is tested with, so the two stay symmetric.
func TestParse_Collection_Mod11_RoundTrip(t *testing.T) {
	barcode := buildCollectionBarcode(t, '8', 4599, "12345678901234567890123456789")
	if barcode[2] != '8' {
		t.Fatalf("built barcode has value type %q, want '8'", barcode[2])
	}

	b, err := Parse(barcode)
	if err != nil {
		t.Fatalf("Parse(barcode) error = %v", err)
	}
	if b.AmountCents != 4599 {
		t.Errorf("AmountCents = %d, want 4599", b.AmountCents)
	}

	line := collectionDigitableLine(t, barcode)
	fromLine, err := Parse(line)
	if err != nil {
		t.Fatalf("Parse(linha digitável) error = %v", err)
	}
	if fromLine.Barcode != barcode {
		t.Errorf("recovered barcode = %s, want %s", fromLine.Barcode, barcode)
	}
}

// TestParse_Collection_Mod11_TamperedCheckDigits keeps the fix from turning
// into "accept anything": a mod11 document with a corrupted digit must
// still be rejected, and with the error that names which digit broke.
func TestParse_Collection_Mod11_TamperedCheckDigits(t *testing.T) {
	t.Run("field digit", func(t *testing.T) {
		line := []byte(gpsDigitableLine)
		line[11] = '0' + (line[11]-'0'+1)%10
		if _, err := Parse(string(line)); !errors.Is(err, ErrInvalidFieldCheckDigit) {
			t.Errorf("expected ErrInvalidFieldCheckDigit, got %v", err)
		}
	})

	t.Run("general digit", func(t *testing.T) {
		barcode := []byte(gpsBarcode)
		barcode[3] = '0' + (barcode[3]-'0'+1)%10
		if _, err := Parse(string(barcode)); !errors.Is(err, ErrInvalidGeneralCheckDigit) {
			t.Errorf("expected ErrInvalidGeneralCheckDigit, got %v", err)
		}
	})
}

// TestParse_Collection_Mod10_StillUsesMod10 is the no-regression guard for
// the variant that already worked: a '6' document must keep validating with
// módulo 10, and must be rejected when checked against módulo 11 digits.
func TestParse_Collection_Mod10_StillUsesMod10(t *testing.T) {
	barcode := buildCollectionBarcode(t, '6', 7500, "12345678901234567890123456789")
	line := collectionDigitableLine(t, barcode)

	b, err := Parse(line)
	if err != nil {
		t.Fatalf("Parse(linha digitável mod10) error = %v", err)
	}
	if b.Barcode != barcode {
		t.Errorf("recovered barcode = %s, want %s", b.Barcode, barcode)
	}

	// The same fields carrying mod11 digits must not pass as a mod10
	// document: the rule really is selected, not guessed.
	mixed := ""
	for _, f := range [4]string{barcode[0:11], barcode[11:22], barcode[22:33], barcode[33:44]} {
		mixed += f + string(mod11Collection(f))
	}
	if _, err := Parse(mixed); !errors.Is(err, ErrInvalidFieldCheckDigit) {
		t.Errorf("expected ErrInvalidFieldCheckDigit for mod11 digits on a mod10 document, got %v", err)
	}
}
