package boleto

import (
	"errors"
	"strconv"
	"time"
)

// ErrInvalidBankCode/ErrInvalidFreeField/ErrDueDateOutOfRange/
// ErrAmountOutOfRange/ErrInvalidSegment guard the builders' inputs.
//
// ErrDueDateOutOfRange names the two representable windows in its message
// because the gap between them is not an off-by-one a caller can nudge past:
// it is the dual-epoch scheme itself. See encodeDueDateFactor.
var (
	ErrInvalidBankCode  = errors.New("bank code must have 3 digits")
	ErrInvalidFreeField = errors.New(
		"free field must have 25 digits for a bank slip or 29 for a collection document")
	ErrDueDateOutOfRange = errors.New(
		"due date is outside the range representable by the due date factor " +
			"(1997-10-07..2000-07-02 or 2025-02-22..2049-10-13)")
	ErrAmountOutOfRange = errors.New("amount is negative or too large for the barcode value field")
	ErrInvalidSegment   = errors.New("collection segment must be a digit 1-9")
)

// maxBankSlipAmountCents and maxUtilityBillAmountCents are the largest
// amounts the respective barcode value fields can hold: 10 digits for a bank
// slip, 11 for a collection document. A value wider than its field would
// push every digit after it out of position, producing a 45-digit string
// that is not a barcode at all rather than a barcode with a wrong amount.
const (
	maxBankSlipAmountCents    = 9999999999
	maxUtilityBillAmountCents = 99999999999
)

// allDigits reports whether s consists solely of ASCII digits.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// encodeDueDateFactor converts dueDate into a barcode "fator de vencimento",
// choosing whichever epoch actually represents it per the scheme documented
// on decodeDueDateFactor. Every date this package's callers construct today
// falls on or after dueDateNewEpoch, and therefore always encodes under the
// new base; the old epoch is only reachable for a date that predates the
// range FEBRABAN reassigned, kept for completeness rather than expected use.
//
// Dates from 2000-07-03 to 2025-02-21 are deliberately refused rather than
// encoded. Under the original scheme they occupied factors 1000-9999, which
// FEBRABAN reassigned to the new base; emitting one would produce a barcode
// that decodeDueDateFactor and every other conforming reader reads back as a
// date in 2025 or later. Refusing is the only answer that does not silently
// corrupt the date, so the gap is a property of the scheme, not a missing
// branch here.
func encodeDueDateFactor(dueDate time.Time) (int, error) {
	d := dueDate.UTC().Truncate(24 * time.Hour)

	if !d.Before(dueDateNewEpoch) {
		factor := dueDateFactorNewBase + int(d.Sub(dueDateNewEpoch).Hours()/24)
		if factor > dueDateFactorMax {
			return 0, ErrDueDateOutOfRange
		}
		return factor, nil
	}

	factor := int(d.Sub(dueDateEpoch).Hours() / 24)
	if factor < 0 || factor >= dueDateFactorNewBase {
		return 0, ErrDueDateOutOfRange
	}
	return factor, nil
}

// BuildBankSlipBarcode assembles a structurally valid, correctly
// check-digited 44-digit bank slip barcode from its parts. It is the
// encoding counterpart to Parse, and the standard way for a caller (this
// package's own tests, or any other package's) to construct a known-valid
// instrument without duplicating the FEBRABAN check-digit algorithm. dueDate may be nil
// (encoded as the FEBRABAN "sem vencimento" factor, 0000). freeField must be
// exactly 25 digits (the assignor-specific portion of the instrument,
// meaningless to this package beyond its length). amountCents must fit the
// 10-digit value field.
func BuildBankSlipBarcode(
	bankCode string,
	dueDate *time.Time,
	amountCents int64,
	freeField string,
) (string, error) {
	if len(bankCode) != 3 {
		return "", ErrInvalidBankCode
	}
	if len(freeField) != 25 {
		return "", ErrInvalidFreeField
	}
	if !allDigits(bankCode) || !allDigits(freeField) {
		return "", ErrNonNumeric
	}
	if amountCents < 0 || amountCents > maxBankSlipAmountCents {
		return "", ErrAmountOutOfRange
	}

	factor := 0
	if dueDate != nil {
		var err error
		factor, err = encodeDueDateFactor(*dueDate)
		if err != nil {
			return "", err
		}
	}
	factorStr := padLeft(strconv.Itoa(factor), 4)
	valueStr := padLeft(strconv.FormatInt(amountCents, 10), 10)

	withoutDV := bankCode + "9" + "?" + factorStr + valueStr + freeField
	dv := computeGeneralCheckDigitMod11(withoutDV)

	return bankCode + "9" + string(dv) + factorStr + valueStr + freeField, nil
}

// BuildUtilityBillBarcode assembles a structurally valid, correctly
// check-digited 44-digit collection ("arrecadação") barcode: the encoding
// counterpart to Parse for the kind BuildBankSlipBarcode does not cover.
//
// segment is the FEBRABAN "segmento" digit at position 2, identifying the
// kind of assignor (1 prefeituras, 2 saneamento, 3 energia e gás, 4
// telecomunicações, 5 órgãos governamentais, 6 carnês, 7 multas de trânsito,
// 9 uso exclusivo do banco). valueType is the "identificador de valor efetivo
// ou referência" at position 3, which selects the check digit rule: '6'
// (módulo 10) and '8' (módulo 11) are supported, and the "quantidade de
// moeda" variants '7' and '9' are refused for the same reason Parse refuses
// them. See collectionCheckDigit. freeField must be exactly 29 digits, and
// amountCents must fit the 11-digit value field.
//
// A collection document has no "fator de vencimento" field, so there is no
// due date to pass: the corresponding Boleto always parses back with a nil
// DueDate.
func BuildUtilityBillBarcode(
	segment byte,
	valueType byte,
	amountCents int64,
	freeField string,
) (string, error) {
	if segment < '1' || segment > '9' {
		return "", ErrInvalidSegment
	}
	checkDigit, ok := collectionCheckDigit(valueType)
	if !ok {
		return "", ErrUnsupportedUtilityBillVariant
	}
	if len(freeField) != 29 {
		return "", ErrInvalidFreeField
	}
	if !allDigits(freeField) {
		return "", ErrNonNumeric
	}
	if amountCents < 0 || amountCents > maxUtilityBillAmountCents {
		return "", ErrAmountOutOfRange
	}

	valueStr := padLeft(strconv.FormatInt(amountCents, 10), 11)
	head := string([]byte{'8', segment, valueType})

	// collectionGeneralCheckDigit reads positions 0-2 and 4-43, so the
	// placeholder at index 3 is never part of the sum.
	dv := collectionGeneralCheckDigit(head+"?"+valueStr+freeField, checkDigit)

	return head + string(dv) + valueStr + freeField, nil
}

func padLeft(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}
