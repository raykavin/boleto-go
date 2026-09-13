# boleto-go

[![Go Reference](https://pkg.go.dev/badge/github.com/raykavin/boleto-go.svg)](https://pkg.go.dev/github.com/raykavin/boleto-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Leitura e validação offline de instrumentos de pagamento brasileiros: boletos
bancários de cobrança e documentos de arrecadação (contas de consumo, tributos,
guias como a GPS).

O pacote decodifica um código de barras ou uma linha digitável no padrão
FEBRABAN e confere todos os dígitos verificadores localmente. Não consulta
banco, registradora nem qualquer serviço de rede.

## Recursos

- Interpreta códigos de barras de 44 dígitos, linhas digitáveis de boleto
  (47 dígitos) e de arrecadação (48 dígitos).
- Confere o dígito verificador geral e, nas linhas digitáveis, o DV de cada
  campo (módulo 10 ou módulo 11, conforme o documento).
- Extrai código do banco, data de vencimento e valor em centavos.
- Trata a virada do fator de vencimento de 2025 (veja
  [Data de vencimento](#data-de-vencimento)).
- Monta códigos de barras de boleto (`BuildBankSlipBarcode`) e de arrecadação
  (`BuildUtilityBillBarcode`), e gera a linha digitável de ambos
  (`DigitableLine`).
- Sem dependências além da biblioteca padrão.

## Requisitos

Go 1.26 ou superior.

> **Atenção:** este repositório ainda não possui um `go.mod`. Hoje ele compila
> como pacote dentro de um módulo pai. Para consumi-lo como módulo independente,
> rode antes `go mod init github.com/raykavin/boleto-go`.

## Instalação

```sh
go get github.com/raykavin/boleto-go
```

```go
import "github.com/raykavin/boleto-go"
```

O nome do pacote é `boleto`.

## Uso

### Leitura

```go
package main

import (
	"fmt"
	"log"

	"github.com/raykavin/boleto-go"
)

func main() {
	b, err := boleto.Parse("23790.00009 00012.345674 89012.345677 5 16000000015075")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(b.Kind)        // 1 (KindBankSlip)
	fmt.Println(b.Barcode)     // 23795160000000150750000000012345678901234567
	fmt.Println(b.BankCode)    // 237
	fmt.Println(*b.DueDate)    // 2026-10-15 00:00:00 +0000 UTC
	fmt.Println(b.AmountCents) // 15075
}
```

`Parse` aceita o mesmo documento como código de barras, como linha digitável, ou
em qualquer uma das duas formas com os separadores `.`, ` ` e `-` usados na
impressão. As quatro chamadas abaixo são equivalentes duas a duas:

```go
boleto.Parse("23795160000000150750000000012345678901234567")    // código de barras
boleto.Parse("23790000090001234567489012345677516000000015075") // linha digitável
boleto.Parse("23790.00009 00012.345674 89012.345677 5 16000000015075")
boleto.Parse("8580-0000.1230 61603852626107162624738599731022") // guia GPS
```

### Documentos de arrecadação

Um documento de arrecadação não carrega código de banco nem campo de
vencimento:

```go
b, _ := boleto.Parse("858000001239061603852620610716262474385997310225")

fmt.Println(b.Kind)        // 2 (KindUtilityBill)
fmt.Println(b.BankCode)    // "", sempre vazio para este tipo
fmt.Println(b.DueDate)     // <nil>, sempre nil para este tipo
fmt.Println(b.AmountCents) // 1230616
```

### Montagem de um código de barras

```go
vencimento := time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)

codigo, err := boleto.BuildBankSlipBarcode(
	"237",                       // código do banco, exatamente 3 dígitos
	&vencimento,                 // nil gera o fator "sem vencimento" (0000)
	15075,                       // valor em centavos
	"0000000012345678901234567", // campo livre, exatamente 25 dígitos
)
// 23795160000000150750000000012345678901234567
```

`BuildBankSlipBarcode` é a contraparte de `Parse`: calcula o dígito verificador
geral, permitindo montar um documento comprovadamente válido sem reimplementar o
algoritmo da FEBRABAN.

Para arrecadação, `BuildUtilityBillBarcode` recebe o segmento e o identificador
de valor no lugar do código do banco e do vencimento:

```go
codigo, err := boleto.BuildUtilityBillBarcode(
	'3',                           // segmento: energia elétrica e gás
	'6',                           // valor efetivo em reais, módulo 10
	12750,                         // valor em centavos
	"12345678901234567890123456789", // campo livre, exatamente 29 dígitos
)
// 83650000001275012345678901234567890123456789
```

### Geração da linha digitável

`DigitableLine` faz o caminho inverso do `Parse`, aceitando tanto um código de
barras quanto uma linha digitável já pronta:

```go
linha, err := boleto.DigitableLine("23795160000000150750000000012345678901234567")
// 23790000090001234567489012345677516000000015075

linha, err = boleto.DigitableLine("83650000001275012345678901234567890123456789")
// 836500000010275012345672890123456786901234567898
```

A entrada passa por `Parse` antes de ser renderizada, então a linha só é gerada
para um documento cujos dígitos verificadores já conferem. O mesmo está
disponível como método de um `Boleto` já lido:

```go
b, _ := boleto.Parse(entrada)
linha, err := b.DigitableLine()
```

A saída não traz os separadores `.` e ` `: eles pertencem à impressão do
documento, não ao seu valor.

## Formatos suportados

| Tamanho | Documento                         | DVs conferidos                         |
| ------- | --------------------------------- | -------------------------------------- |
| 44      | Código de barras (ambos os tipos) | Geral                                  |
| 47      | Linha digitável de boleto         | 3 DVs de campo (mód. 10) + geral       |
| 48      | Linha digitável de arrecadação    | 4 DVs de campo (mód. 10 ou 11) + geral |

Boleto e arrecadação são distinguidos pela posição 1 do código de barras: a
arrecadação sempre traz o identificador de produto fixo `8`, enquanto o boleto
traz ali o primeiro dígito do código do banco.

Na arrecadação, a posição 3 (o _identificador de valor efetivo ou referência_)
define tanto a regra de DV quanto a leitura do campo de valor:

| Dígito | Campo de valor         | Regra     | Suportado |
| ------ | ---------------------- | --------- | --------- |
| `6`    | Valor efetivo em reais | Módulo 10 | Sim       |
| `7`    | Quantidade de moeda    | Módulo 10 | Não       |
| `8`    | Valor efetivo em reais | Módulo 11 | Sim       |
| `9`    | Quantidade de moeda    | Módulo 11 | Não       |

As duas variantes de _quantidade de moeda_ são recusadas com
`ErrUnsupportedUtilityBillVariant`: o campo de valor conta unidades de uma moeda
de referência, não centavos, e decodificá-lo como dinheiro seria incorreto.

## API

### `Parse(input string) (*Boleto, error)`

Normaliza os separadores, valida a entrada e devolve o documento decodificado.

### `Boleto`

| Campo         | Tipo         | Observações                                                                                         |
| ------------- | ------------ | --------------------------------------------------------------------------------------------------- |
| `Kind`        | `Kind`       | `KindBankSlip` (1) ou `KindUtilityBill` (2)                                                         |
| `Barcode`     | `string`     | Sempre o código de barras normalizado de 44 dígitos, mesmo quando a entrada foi uma linha digitável |
| `BankCode`    | `string`     | 3 dígitos no boleto; `""` na arrecadação                                                            |
| `DueDate`     | `*time.Time` | `nil` quando ausente (fator `0000`, ou qualquer arrecadação)                                        |
| `AmountCents` | `int64`      | Valor em centavos                                                                                   |
| `FreeField`   | `string`     | 25 dígitos no boleto, 29 na arrecadação; conteúdo específico do cedente                             |

### `BuildBankSlipBarcode(bankCode string, dueDate *time.Time, amountCents int64, freeField string) (string, error)`

Monta um código de barras de boleto de 44 dígitos com o DV geral correto.
`bankCode` tem 3 dígitos, `freeField` tem 25, `dueDate` pode ser nil e
`amountCents` precisa caber no campo de valor de 10 dígitos.

### `BuildUtilityBillBarcode(segment, valueType byte, amountCents int64, freeField string) (string, error)`

Monta um código de barras de arrecadação de 44 dígitos. `segment` é o dígito de
segmento (`'1'` a `'9'`), `valueType` é o identificador de valor (`'6'` ou
`'8'`), `freeField` tem 29 dígitos e `amountCents` precisa caber no campo de
valor de 11 dígitos.

### `DigitableLine(input string) (string, error)`

Renderiza a linha digitável de um documento: 47 dígitos para boleto, 48 para
arrecadação. Aceita código de barras ou linha digitável como entrada.

### `(*Boleto) DigitableLine() (string, error)`

O mesmo, a partir de um `Boleto` já lido.

## Erros

Todos os erros são valores sentinela, comparáveis com `errors.Is`.

| Erro | Causa |
| --- | --- |
| `ErrEmptyInput` | Entrada vazia, ou composta apenas dos separadores `.` ` ` `-` |
| `ErrInvalidLength` | A entrada não tem 44, 47 nem 48 dígitos |
| `ErrNonNumeric` | A entrada contém algum caractere que não é dígito nem separador |
| `ErrInvalidFieldCheckDigit` | O DV de um campo da linha digitável não confere |
| `ErrInvalidGeneralCheckDigit` | O DV geral do código de barras não confere |
| `ErrUnsupportedUtilityBillVariant` | Identificador de valor da arrecadação é `7`, `9`, ou está fora da tabela FEBRABAN |
| `ErrInvalidBankCode` | `BuildBankSlipBarcode`: código do banco não tem 3 dígitos |
| `ErrInvalidSegment` | `BuildUtilityBillBarcode`: segmento fora da faixa `'1'` a `'9'` |
| `ErrInvalidFreeField` | Campo livre não tem 25 dígitos (boleto) ou 29 (arrecadação) |
| `ErrAmountOutOfRange` | Valor negativo, ou largo demais para o campo de valor |
| `ErrDueDateOutOfRange` | `BuildBankSlipBarcode`: data não representável pelo fator de 4 dígitos |

```go
if _, err := boleto.Parse(entrada); errors.Is(err, boleto.ErrInvalidGeneralCheckDigit) {
	// código de barras malformado ou digitado errado
}
```

## Data de vencimento

O boleto codifica o vencimento num _fator de vencimento_ de 4 dígitos: a
quantidade de dias desde uma data-base. A base original (07/10/1997) esgotou o
campo em `9999` no dia 21/02/2025. Em vez de ampliar o campo, a FEBRABAN
reiniciou o contador em `1000` a partir da nova base de 22/02/2025.

O pacote decodifica as duas: fator `1000` ou maior conta a partir de 22/02/2025;
fator abaixo de `1000`, a partir de 07/10/1997. O fator `0000` significa _sem
vencimento_ e resulta em `DueDate` nil. As datas são devolvidas em UTC, à
meia-noite.

Na prática, a nova base cobre de 22/02/2025 a 13/10/2049.

## Regras de dígito verificador

As três variantes ficam em [`checkdigit.go`](checkdigit.go):

- **`mod10`**: pesos alternados 2 e 1 da direita para a esquerda, com produtos
  ≥ 10 reduzidos em 9. Usada nos campos da linha digitável do boleto e na
  arrecadação com identificador `6`.
- **`mod11`**: pesos cíclicos de 2 a 9; resto 0 ou 1 resulta em DV `1`. Usada no
  DV geral do boleto.
- **`mod11Collection`**: mesma pesagem do `mod11`, mas resto 0 ou 1 resulta em
  `0` em vez de `1`. A arrecadação realmente difere nesse ponto, então a regra do
  boleto não pode ser reaproveitada; os testes fixam esse comportamento com uma
  guia GPS real.

## Desenvolvimento

```sh
go test ./...          # roda os testes
go test -cover ./...   # com cobertura
go vet ./...
gofmt -l .
```

Os testes são de ida e volta: montam um documento válido, leem de volta e
comparam; em seguida alteram dígitos isolados para provar que o leitor rejeita
corrupção em vez de aceitar qualquer coisa. A guia GPS em
[`boleto_test.go`](boleto_test.go) é um documento real mantido como teste de
regressão.

### Contribuindo

1. Abra uma issue descrevendo o documento ou a variante envolvida.
2. Escreva o teste primeiro. Em caso de falha de leitura, inclua o código de
   barras ou a linha digitável real que a reproduz.
3. Mantenha `gofmt -l .` e `go vet ./...` limpos.

## Referências

- FEBRABAN: _Layout de Código de Barras para Cobrança_
- FEBRABAN: _Layout Padrão de Arrecadação / Recebimento com Utilização do
  Código de Barras_
- FEBRABAN: comunicado sobre o reinício do fator de vencimento (2025)

## Licença

[MIT](LICENSE) © raykavin
