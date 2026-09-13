package boleto

import (
	"errors"
	"strconv"
	"time"
)

// ErrInvalidBankCode/ErrInvalidFreeField/ErrDueDateOutOfRange guard
// BuildBankSlipBarcode's inputs.
var (
	ErrInvalidBankCode   = errors.New("bank code must have 3 digits")
	ErrInvalidFreeField  = errors.New("free field must have 25 digits")
	ErrDueDateOutOfRange = errors.New("due date is outside the range representable by the due date factor")
)

// encodeDueDateFactor converts dueDate into a barcode "fator de vencimento",
// choosing whichever epoch actually represents it per the scheme documented
// on decodeDueDateFactor. Every date this package's callers construct today
// falls on or after dueDateNewEpoch, and therefore always encodes under the
// new base; the old epoch is only reachable for a date that predates the
// range FEBRABAN reassigned, kept for completeness rather than expected use.
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
// check-digited 44-digit bank slip barcode from its parts the encoding
// counterpart to Parse, and the standard way for a caller (this package's
// own tests, or any other package's) to construct a known-valid instrument
// without duplicating the FEBRABAN check-digit algorithm. dueDate may be nil
// (encoded as the FEBRABAN "sem vencimento" factor, 0000). freeField must be
// exactly 25 digits (the assignor-specific portion of the instrument,
// meaningless to this package beyond its length).
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
	for _, s := range []string{bankCode, freeField} {
		for _, r := range s {
			if r < '0' || r > '9' {
				return "", ErrNonNumeric
			}
		}
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

func padLeft(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}
