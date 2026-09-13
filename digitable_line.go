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
// barcode's own general digit see collectionCheckDigit. The rule is read
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
