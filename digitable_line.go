package boleto

// digitableLineToBarcode47 validates and converts a 47-digit bank slip
// linha digitável into its 44-digit barcode. Layout (0-indexed):
//
//	Campo 1 (10): data = input[0:9]  (bankCode[3]+currency[1]+freeField[0:5]), DV1 = input[9]  (mod10)
//	Campo 2 (11): data = input[10:20] (freeField[5:15]),                       DV2 = input[20] (mod10)
//	Campo 3 (11): data = input[21:31] (freeField[15:25]),                      DV3 = input[31] (mod10)
//	Campo 4 (1):  general check digit (barcode position 5)  = input[32]
//	Campo 5 (14): fator de vencimento (4) + valor (10)       = input[33:47]
func digitableLineToBarcode47(input string) (string, error) {
	if len(input) != 47 {
		return "", ErrInvalidLength
	}

	data1, dv1 := input[0:9], input[9]
	data2, dv2 := input[10:20], input[20]
	data3, dv3 := input[21:31], input[31]
	generalDV := input[32]
	fatorEValor := input[33:47]

	if mod10(data1) != dv1 || mod10(data2) != dv2 || mod10(data3) != dv3 {
		return "", ErrInvalidFieldCheckDigit
	}

	bankAndCurrency := data1[0:4]
	freeField := data1[4:9] + data2 + data3

	barcode := bankAndCurrency + string(generalDV) + fatorEValor + freeField
	if len(barcode) != 44 {
		return "", ErrInvalidLength
	}
	return barcode, nil
}

// digitableLineToBarcode48 validates and converts a 48-digit collection
// ("arrecadação") linha digitável into its 44-digit barcode. Layout: four
// 12-character fields, each 11 barcode data digits followed by their own
// check digit.
//
// That check digit follows módulo 10 or módulo 11 according to the
// document's value-type digit, the same digit that selects the rule for the
// barcode's own general digit. See collectionCheckDigit. The rule is read
// once, up front, and applied to all four fields: they are four slices of
// one document, never a mix.
func digitableLineToBarcode48(input string) (string, error) {
	if len(input) != 48 {
		return "", ErrInvalidLength
	}

	// The value-type digit sits at position 3 of the barcode, which is also
	// position 3 of the line: the first field's 11 data digits are the
	// barcode's own first 11, so the digit survives at the same index. It
	// has to be read before the fields are checked, because it is what says
	// which rule to check them with.
	checkDigit, ok := collectionCheckDigit(input[2])
	if !ok {
		return "", ErrUnsupportedUtilityBillVariant
	}

	fields := [4][2]int{{0, 12}, {12, 24}, {24, 36}, {36, 48}}
	barcode := make([]byte, 0, 44)
	for _, f := range fields {
		segment := input[f[0]:f[1]]
		data, dv := segment[0:11], segment[11]
		if checkDigit(data) != dv {
			return "", ErrInvalidFieldCheckDigit
		}
		barcode = append(barcode, data...)
	}
	return string(barcode), nil
}

// DigitableLine renders a document as the linha digitável printed on it: 47
// digits for a bank slip, 48 for a collection document. It is the inverse of
// the two converters above, and completes the round trip in the direction
// Parse does not cover.
//
// input may be a 44-digit barcode or an existing linha digitável; it is run
// through Parse first, so the result is only ever produced for a document
// whose check digits already verify. The output carries no '.' or ' '
// separators: those belong to how a document is typeset, not to its value.
func DigitableLine(input string) (string, error) {
	b, err := Parse(input)
	if err != nil {
		return "", err
	}
	return b.DigitableLine()
}

// DigitableLine renders an already-parsed Boleto as its linha digitável.
// A Boleto only ever exists having passed Parse, so the barcode it carries
// is known-valid and the field check digits are computed, never re-verified.
func (b *Boleto) DigitableLine() (string, error) {
	if len(b.Barcode) != 44 {
		return "", ErrInvalidLength
	}
	if b.Kind == KindUtilityBill {
		return collectionDigitableLine(b.Barcode)
	}
	return bankSlipDigitableLine(b.Barcode), nil
}

// bankSlipDigitableLine lays a 44-digit bank slip barcode out as the 47
// digits of its linha digitável, mirroring digitableLineToBarcode47's field
// map in reverse: three mod10-checked fields, then the barcode's own general
// check digit, then the fator de vencimento and valor verbatim.
func bankSlipDigitableLine(barcode string) string {
	data1 := barcode[0:4] + barcode[19:24] // bank + currency + free field head
	data2 := barcode[24:34]
	data3 := barcode[34:44]

	return data1 + string(mod10(data1)) +
		data2 + string(mod10(data2)) +
		data3 + string(mod10(data3)) +
		barcode[4:5] + // general check digit
		barcode[5:19] // fator de vencimento + valor
}

// collectionDigitableLine lays a 44-digit collection barcode out as the 48
// digits of its linha digitável: four 11-digit slices, each followed by its
// own check digit under the rule the document's value-type digit selects.
// The rule is read once and applied to all four, exactly as
// digitableLineToBarcode48 verifies them.
func collectionDigitableLine(barcode string) (string, error) {
	checkDigit, ok := collectionCheckDigit(barcode[2])
	if !ok {
		return "", ErrUnsupportedUtilityBillVariant
	}

	line := make([]byte, 0, 48)
	for i := 0; i < 44; i += 11 {
		field := barcode[i : i+11]
		line = append(line, field...)
		line = append(line, checkDigit(field))
	}
	return string(line), nil
}
