// Package boleto parses and validates Brazilian bank slips and utility bills.
// It verifies FEBRABAN barcodes and digitable lines without querying a bank.
package boleto

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Kind identifies the FEBRABAN barcode layout.
type Kind int8

const (
	// KindBankSlip is a standard bank slip ("boleto bancário de cobrança"),
	// discriminated by barcode position 1 (1-indexed) NOT being the fixed
	// convênio product identifier '8'; position 4 is always '9' (Real
	// currency), and the general check digit (position 5) is computed with
	// mod11.
	KindBankSlip Kind = iota + 1
	// KindUtilityBill is a collection document ("arrecadação": conta de
	// consumo/convênio água, luz, gás, tributos, guias como a GPS),
	// discriminated by barcode position 1 being the fixed product
	// identifier '8'. Position 3 (the "identificador de valor efetivo ou
	// referência") then determines whether the check digits use mod10 or
	// mod11 and whether the value field holds centavos or a quantity of
	// reference currency; both mod10 and mod11 are validated, while the
	// quantity-of-currency variants are refused. See
	// ErrUnsupportedUtilityBillVariant and collectionCheckDigit.
	KindUtilityBill
)

// dueDateEpoch is the original FEBRABAN reference date (1997-10-07) that a
// bank slip's 4-digit "fator de vencimento" counted days from. The 4-digit
// field saturates at 9999, which this epoch reaches on 2025-02-21; FEBRABAN
// addressed that industry-wide by resetting the counter to
// dueDateFactorNewBase (1000) starting the next day, from the new
// dueDateNewEpoch (2025-02-22) instead of extending the field. Any barcode
// or linha digitável carrying a due date from 2025-02-22 onward (which, as
// of this package's current use, is every one still being issued or
// received) uses the new base; only a factor below dueDateFactorNewBase
// still refers to the original 1997 base.
var (
	dueDateEpoch    = time.Date(1997, time.October, 7, 0, 0, 0, 0, time.UTC)
	dueDateNewEpoch = time.Date(2025, time.February, 22, 0, 0, 0, 0, time.UTC)
)

// dueDateFactorNewBase is the first "fator de vencimento" value that refers
// to dueDateNewEpoch rather than dueDateEpoch. dueDateFactorMax is the
// largest value the 4-digit field can hold.
const (
	dueDateFactorNewBase = 1000
	dueDateFactorMax     = 9999
)

// decodeDueDateFactor converts a barcode's 4-digit "fator de vencimento"
// into the calendar date it represents, per the dual-epoch scheme above.
func decodeDueDateFactor(factor int) time.Time {
	if factor >= dueDateFactorNewBase {
		return dueDateNewEpoch.AddDate(0, 0, factor-dueDateFactorNewBase)
	}
	return dueDateEpoch.AddDate(0, 0, factor)
}

var (
	// ErrEmptyInput is returned for an empty barcode/linha digitável.
	ErrEmptyInput = errors.New("empty barcode or digitable line")
	// ErrInvalidLength is returned when the input is not 44 (barcode), 47
	// (bank slip linha digitável) or 48 (utility bill linha digitável)
	// digits long.
	ErrInvalidLength = errors.New("invalid barcode or digitable line length")
	// ErrNonNumeric is returned when the input contains anything besides
	// digits and the conventional '.', ' ', '-' separators a linha digitável
	// is typeset with.
	ErrNonNumeric = errors.New("barcode or digitable line must contain only digits")
	// ErrInvalidFieldCheckDigit is returned when a linha digitável field
	// check digit doesn't match its field: one of a bank slip's three
	// (DAC1/DAC2/DAC3, mod10), or one of a collection document's four
	// (mod10 or mod11, per its value-type digit).
	ErrInvalidFieldCheckDigit = errors.New("invalid digitable line field check digit")
	// ErrInvalidGeneralCheckDigit is returned when the barcode's own general
	// check digit (position 5, mod11) doesn't match.
	ErrInvalidGeneralCheckDigit = errors.New("invalid barcode general check digit")
	// ErrUnsupportedUtilityBillVariant is returned for a collection
	// document whose value-type digit is one this package does not decode:
	// the "quantidade de moeda" variants, whose value field is not centavos,
	// or a digit outside FEBRABAN's table altogether. Both módulo 10 and
	// módulo 11 documents are supported see collectionCheckDigit.
	ErrUnsupportedUtilityBillVariant = errors.New("unsupported utility bill variant")
)

// Boleto contains a locally validated barcode or digitable line.
type Boleto struct {
	Kind        Kind
	Barcode     string // always the normalized 44-digit barcode
	BankCode    string // 3 digits, KindBankSlip only ("" for KindUtilityBill)
	DueDate     *time.Time
	AmountCents int64
	FreeField   string
}

// Parse validates a 44-digit barcode or a 47/48-digit digitable line.
// It normalizes separators and extracts the encoded due date and amount.
func Parse(input string) (*Boleto, error) {
	digits, ok := onlyDigits(input)
	if !ok {
		return nil, ErrNonNumeric
	}
	if digits == "" {
		return nil, ErrEmptyInput
	}

	switch len(digits) {
	case 44:
		return parseBarcode(digits)
	case 47:
		barcode, err := digitableLineToBarcode47(digits)
		if err != nil {
			return nil, err
		}
		return parseBarcode(barcode)
	case 48:
		barcode, err := digitableLineToBarcode48(digits)
		if err != nil {
			return nil, err
		}
		return parseBarcode(barcode)
	default:
		return nil, ErrInvalidLength
	}
}

// onlyDigits strips the '.', ' ' and '-' conventionally used to typeset a
// linha digitável for humans, and reports whether the input consisted of
// nothing else. Anything further, such as a letter, a symbol or stray
// punctuation, makes it return false, so Parse can answer ErrNonNumeric
// rather than silently discarding the offending rune. An all-separator input is not an error
// here: it yields ("", true), which Parse reports as ErrEmptyInput.
func onlyDigits(input string) (string, bool) {
	var sb strings.Builder
	for _, r := range input {
		switch {
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '.' || r == ' ' || r == '-':
			continue
		default:
			return "", false
		}
	}
	return sb.String(), true
}

// parseBarcode validates and decodes an already-normalized 44-digit barcode.
func parseBarcode(barcode string) (*Boleto, error) {
	if len(barcode) != 44 {
		return nil, ErrInvalidLength
	}
	for _, r := range barcode {
		if r < '0' || r > '9' {
			return nil, ErrNonNumeric
		}
	}

	// FEBRABAN discriminator: a utility-bill ("convênio") barcode always
	// starts with the fixed product identifier '8'; a bank slip's first 3
	// digits are its bank code instead (never '8xx' by convention '8' is
	// reserved for convênios), followed by the currency digit ('9' = Real)
	// at position 4. Checking position 1 first is the only reliable
	// discriminator: position 4 alone cannot be used, since for a utility
	// bill it holds that barcode's own check digit, which can coincidentally
	// take any value including '9'.
	if barcode[0] == '8' {
		return parseUtilityBillBarcode(barcode)
	}
	return parseBankSlipBarcode(barcode)
}

// parseBankSlipBarcode validates a KindBankSlip barcode: general check
// digit at position 5 (mod11 over the other 43 digits), due-date factor at
// positions 6-9 (days since dueDateEpoch, 0000 meaning "no due date"), and
// value at positions 10-19 (cents).
func parseBankSlipBarcode(barcode string) (*Boleto, error) {
	bankCode := barcode[0:3]
	generalDV := barcode[4]
	dueDateFactor := barcode[5:9]
	valueField := barcode[9:19]
	freeField := barcode[19:44]

	if computeGeneralCheckDigitMod11(barcode) != generalDV {
		return nil, ErrInvalidGeneralCheckDigit
	}

	amount, err := strconv.ParseInt(valueField, 10, 64)
	if err != nil {
		return nil, ErrNonNumeric
	}

	var dueDate *time.Time
	if factor, ferr := strconv.Atoi(dueDateFactor); ferr == nil && factor > 0 {
		d := decodeDueDateFactor(factor)
		dueDate = &d
	}

	return &Boleto{
		Kind:        KindBankSlip,
		Barcode:     barcode,
		BankCode:    bankCode,
		DueDate:     dueDate,
		AmountCents: amount,
		FreeField:   freeField,
	}, nil
}

// parseUtilityBillBarcode validates a KindUtilityBill barcode. Position 3
// (0-indexed 2) is the "identificador de valor efetivo ou referência",
// which selects both the check digit rule and how the value field reads;
// collectionCheckDigit owns that table and says which variants are
// supported. The value sits at positions 5-15 and the general check digit
// at position 4, computed over the other 43 digits under the selected rule.
// Collection documents have no FEBRABAN due-date factor field, so DueDate
// is always nil.
func parseUtilityBillBarcode(barcode string) (*Boleto, error) {
	checkDigit, ok := collectionCheckDigit(barcode[2])
	if !ok {
		return nil, ErrUnsupportedUtilityBillVariant
	}

	generalDV := barcode[3]
	valueField := barcode[4:15]
	freeField := barcode[15:44]

	if collectionGeneralCheckDigit(barcode, checkDigit) != generalDV {
		return nil, ErrInvalidGeneralCheckDigit
	}

	amount, err := strconv.ParseInt(valueField, 10, 64)
	if err != nil {
		return nil, ErrNonNumeric
	}

	return &Boleto{
		Kind:        KindUtilityBill,
		Barcode:     barcode,
		AmountCents: amount,
		FreeField:   freeField,
	}, nil
}
