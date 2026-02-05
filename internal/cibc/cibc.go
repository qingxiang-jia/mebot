package cibc

import (
	"encoding/json"
	"io"
	"time"
)

type Transaction struct {
	Date        time.Time
	Amount      float64
	Description string
}

type jsonTransaction struct {
	Date        string   `json:"date"`
	Debit       *float64 `json:"debit"`
	Credit      *float64 `json:"credit"`
	Description string   `json:"transactionDescription"`
}

type jsonRoot struct {
	Transactions []jsonTransaction `json:"transactions"`
}

// Read parses the CIBC JSON data and returns a slice of Transactions.
func Read(r io.Reader) ([]Transaction, error) {
	var root jsonRoot
	if err := json.NewDecoder(r).Decode(&root); err != nil {
		return nil, err
	}

	var result []Transaction
	for _, t := range root.Transactions {
		// Parse the date (RFC3339 format based on sample)
		parsedDate, err := time.Parse(time.RFC3339, t.Date)
		if err != nil {
			return nil, err
		}

		var amount float64
		if t.Debit != nil {
			amount = *t.Debit
		} else if t.Credit != nil {
			amount = -*t.Credit // Negate credit as requested
		}

		result = append(result, Transaction{
			Date:        parsedDate,
			Amount:      amount,
			Description: t.Description,
		})
	}
	return result, nil
}
