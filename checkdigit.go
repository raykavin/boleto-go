package boleto

// mod10 computes the FEBRABAN "módulo 10" check digit for a digit string:
// walking right to left, digits are alternately multiplied by 2 and 1
// (starting with 2 on the rightmost digit); any product of 10 or more has
// its digits summed (equivalent to subtracting 9, since the maximum
// product is 18). The check digit is (10 - (sum mod 10)) mod 10.
func mod10(digits string) byte {
	sum := 0
	weight := 2
	for i := len(digits) - 1; i >= 0; i-- {
		product := int(digits[i]-'0') * weight
		if product > 9 {
			product -= 9
		}
		sum += product
		if weight == 2 {
			weight = 1
		} else {
			weight = 2
		}
	}
	remainder := sum % 10
	if remainder == 0 {
		return '0'
	}
	return byte('0' + (10 - remainder))
}

// mod11 computes the FEBRABAN "módulo 11" general check digit used by a
// bank slip barcode: walking right to left, digits are
// multiplied by cyclically repeating weights 2..9, summed, and reduced mod
// 11. A remainder of 0 or 1 maps to check digit 1 (avoiding the invalid,
// two-digit results 11-0=11 and 11-1=10); any other remainder maps to
// 11 - remainder.
//
// A collection document's módulo 11 is a different mapping; see
// mod11Collection, which must not be conflated with this one.
func mod11(digits string) byte {
	sum := 0
	weight := 2
	for i := len(digits) - 1; i >= 0; i-- {
		sum += int(digits[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}
	remainder := sum % 11
	if remainder == 0 || remainder == 1 {
		return '1'
	}
	return byte('0' + (11 - remainder))
}

// mod11Collection computes the "módulo 11" check digit a collection
// document ("arrecadação": utility bills, tax slips, a GPS guide) uses, for
// both its four linha digitável field digits and its barcode's own general
// digit.
//
// The weighting is the same cyclic 2..9 as mod11, but the remainder maps
// differently: here a remainder of 0 or 1 yields check digit 0, where a
// bank slip yields 1. The difference is real and not academic: the GPS
// guide that exposed this carries a field whose remainder is 0 and whose
// printed digit is 0, so mod11 cannot be reused for a collection document.
func mod11Collection(digits string) byte {
	sum := 0
	weight := 2
	for i := len(digits) - 1; i >= 0; i-- {
		sum += int(digits[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}
	if remainder := sum % 11; remainder > 1 {
		return byte('0' + (11 - remainder))
	}
	return '0'
}

// collectionCheckDigit returns the check digit rule selected by a collection
// document's "identificador de valor efetivo ou referência" (barcode
// position 3, 0-indexed 2), and reports whether this package supports that
// variant.
//
// FEBRABAN's table pairs the four values like this:
//
//	'6'  valor efetivo em reais   módulo 10
//	'7'  quantidade de moeda      módulo 10
//	'8'  valor efetivo em reais   módulo 11
//	'9'  quantidade de moeda      módulo 11
//
// The módulo is chosen by 6/7 against 8/9, not by 6/8 against 7/9. This
// package had the table the wrong way round, which made it validate every
// módulo 11 document (a large share of the tax and social security guides
// it exists to read) with módulo 10, and report the mismatch as a malformed
// check digit.
//
// The two "quantidade de moeda" variants stay unsupported: their value
// field counts units of a reference currency rather than centavos, and
// nothing here decodes that, so accepting them would hand the amount rules
// a number that is not money. They are refused for that reason now, rather
// than for a check digit rule they never had.
func collectionCheckDigit(valueTypeDigit byte) (func(string) byte, bool) {
	switch valueTypeDigit {
	case '6':
		return mod10, true
	case '8':
		return mod11Collection, true
	default:
		return nil, false
	}
}

// computeGeneralCheckDigitMod11 computes a bank slip barcode's own general
// check digit (position 5, 1-indexed) from the other 43 digits.
func computeGeneralCheckDigitMod11(barcode string) byte {
	return mod11(barcode[0:4] + barcode[5:44])
}

// collectionGeneralCheckDigit computes a collection barcode's own general
// check digit (position 4, 1-indexed) from the other 43 digits, under the
// rule its value-type digit selected.
func collectionGeneralCheckDigit(barcode string, checkDigit func(string) byte) byte {
	return checkDigit(barcode[0:3] + barcode[4:44])
}
