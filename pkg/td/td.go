package td

import (
	"encoding/csv"
	"io"
	"strconv"
	"time"
)

type Transaction struct {
	Date   time.Time
	Amount float64
}

// Read parses the TD CSV data and returns a slice of Transactions.
// Assumes CSV format: Date, Description, Debit, Credit, Balance
// Date format: MM/DD/YYYY
func Read(r io.Reader) ([]Transaction, error) {
	reader := csv.NewReader(r)
	// Allow variable number of fields if necessary, though sample implies fixed.
	// reader.FieldsPerRecord = -1 

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var result []Transaction
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}

		dateStr := row[0]
		debitStr := row[2]
		creditStr := row[3]

		// Parse date MM/DD/YYYY
		parsedDate, err := time.Parse("01/02/2006", dateStr)
		if err != nil {
			return nil, err
		}

		var amount float64
		if debitStr != "" {
			val, err := strconv.ParseFloat(debitStr, 64)
			if err != nil {
				return nil, err
			}
			amount = val
		} else if creditStr != "" {
			val, err := strconv.ParseFloat(creditStr, 64)
			if err != nil {
				return nil, err
			}
			amount = -val // Negate credit
		}

		result = append(result, Transaction{
			Date:   parsedDate,
			Amount: amount,
		})
	}
	return result, nil
}
