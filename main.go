package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mebot/internal/cibc"
	"mebot/internal/extract"
	"mebot/internal/td"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mebot <command> [wsj|economist|spending]")
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch strings.ToLower(cmd) {
	case "wsj":
		if err := handleReading("WSJ"); err != nil {
			log.Fatalf("Error processing WSJ: %v", err)
		}
	case "economist":
		if err := handleReading("Economist"); err != nil {
			log.Fatalf("Error processing Economist: %v", err)
		}
	case "spending":
		if err := handleSpending(); err != nil {
			log.Fatalf("Error processing spending: %v", err)
		}
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

// --- Daily Reading Logic ---

func handleReading(source string) error {
	// 0. Check if new.md already exists
	if _, err := os.Stat("new.md"); err == nil {
		fmt.Println("Warning: new.md already exists. No changes made.")
		return nil
	}

	// 1. Process HTML files
	files, err := filepath.Glob("*.html")
	if err != nil {
		return fmt.Errorf("failed to glob html files: %w", err)
	}

	hasHTML := len(files) > 0
	hasSummary := false
	if _, err := os.Stat("summary.md"); err == nil {
		hasSummary = true
	}

	if !hasHTML && !hasSummary {
		fmt.Println("No HTML files or summary.md found.")
		return nil
	}

	var articleContent string
	if hasHTML {
		var sb strings.Builder
		processedCount := 0
		for _, file := range files {
			f, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("failed to open %s: %w", file, err)
			}

			text, title, err := extract.FromHTML(f)
			f.Close() // Close explicitly

			if err != nil {
				fmt.Printf("Warning: failed to extract from %s: %v\n", file, err)
				continue
			}

			if text != "" {
				if title != "" {
					fmt.Printf("%s\n", title)
				} else {
					fmt.Printf("Processed: %s (no title found)\n", file)
				}
				sb.WriteString(text)
				sb.WriteString("\n\n")
				processedCount++
			} else {
				fmt.Printf("Warning: no content extracted from %s. Is it a valid article?\n", file)
			}
		}
		articleContent = sb.String()

		if articleContent != "" {
			// Write to new.md
			if err := appendToFile("new.md", articleContent); err != nil {
				return fmt.Errorf("failed to write to new.md: %w", err)
			}
			fmt.Printf("Successfully extracted %d article(s) to new.md\n", processedCount)
		} else {
			fmt.Println("No content extracted from any HTML files.")
		}

		// Move HTML files to deleted
		if err := os.MkdirAll("deleted", 0755); err != nil {
			return fmt.Errorf("failed to create deleted directory: %w", err)
		}

		for _, file := range files {
			if err := moveFile(file, filepath.Join("deleted", file)); err != nil {
				return fmt.Errorf("failed to move %s: %w", file, err)
			}
		}
	}

	var summaryContent string
	if hasSummary {
		b, err := os.ReadFile("summary.md")
		if err == nil {
			summaryContent = extractSummary(string(b))
		}
	}

	// 2. Update YYYY-MM-DD <Source>.md
	if articleContent != "" || summaryContent != "" {
		targetDate := getNextSaturday(time.Now())
		targetFilename := fmt.Sprintf("%s %s.md", targetDate.Format("2006-01-02"), source)

		if err := updateTargetFile(targetFilename, articleContent, summaryContent); err != nil {
			return fmt.Errorf("failed to update %s: %w", targetFilename, err)
		}
	}

	// 3. Process summary.md cleanup
	if hasSummary {
		if err := os.MkdirAll("deleted", 0755); err != nil {
			return fmt.Errorf("failed to create deleted directory: %w", err)
		}
		if err := moveFile("summary.md", filepath.Join("deleted", "summary.md")); err != nil {
			return fmt.Errorf("failed to move summary.md: %w", err)
		}
	}

	return nil
}

func updateTargetFile(filename, articleContent, summaryContent string) error {
	// Read existing content
	content := ""
	if b, err := os.ReadFile(filename); err == nil {
		content = string(b)
	}

	// If file is empty or new
	if content == "" {
		if articleContent != "" {
			content += "# Full Text\n\n" + articleContent + "\n\n"
		}
		if summaryContent != "" {
			content += "# Summary\n\n" + summaryContent + "\n\n"
		}
		return os.WriteFile(filename, []byte(content), 0644)
	}

	// File exists.
	// Check for # Full Text
	if articleContent != "" {
		if strings.Contains(content, "# Full Text") {
			parts := strings.SplitN(content, "# Full Text", 2)
			nextHeaderIdx := strings.Index(parts[1], "\n# ")
			if nextHeaderIdx != -1 {
				pre := parts[1][:nextHeaderIdx]
				post := parts[1][nextHeaderIdx:]
				parts[1] = pre + "\n\n" + articleContent + post
			} else {
				parts[1] = parts[1] + "\n\n" + articleContent
			}
			content = parts[0] + "# Full Text" + parts[1]
		} else {
			// Append Full Text section
			content = content + "\n\n# Full Text\n\n" + articleContent
		}
	}

	// Check for # Summary
	if summaryContent != "" {
		if strings.Contains(content, "# Summary") {
			parts := strings.SplitN(content, "# Summary", 2)
			// Append to the section. Find start of next section or end.
			nextHeaderIdx := strings.Index(parts[1], "\n# ")
			if nextHeaderIdx != -1 {
				pre := parts[1][:nextHeaderIdx]
				post := parts[1][nextHeaderIdx:]
				parts[1] = pre + "\n\n" + summaryContent + post
			} else {
				parts[1] = parts[1] + "\n\n" + summaryContent
			}
			content = parts[0] + "# Summary" + parts[1]
		} else {
			// Append Summary section (now after Full Text if it exists)
			if strings.HasSuffix(content, "\n") {
				content = content + "\n# Summary\n\n" + summaryContent
			} else {
				content = content + "\n\n# Summary\n\n" + summaryContent
			}
		}
	}

	return os.WriteFile(filename, []byte(content), 0644)
}

func extractSummary(raw string) string {
	// Find all level 2 markdown titles and their content
	lines := strings.Split(raw, "\n")
	var sb strings.Builder
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = true
			sb.WriteString(line)
			sb.WriteString("\n")
			continue
		}
		if inSection {
			// If we hit a level 1 header, stop the current section
			if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
				inSection = false
				continue
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	if sb.Len() == 0 {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(sb.String())
}

func getNextSaturday(t time.Time) time.Time {
	// If today is Saturday, do we mean today or next week?
	// "Coming Saturday" usually implies the future.
	// Let's assume if today is Saturday, we mean next Saturday (7 days later).
	// If today is Friday, it's tomorrow.

	daysUntilSaturday := (6 - int(t.Weekday()) + 7) % 7
	if daysUntilSaturday == 0 {
		daysUntilSaturday = 7
	}
	return t.AddDate(0, 0, daysUntilSaturday)
}

func appendToFile(filename, content string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return err
	}
	return nil
}

func moveFile(src, dst string) error {
	// Handle naming conflict
	finalDst := dst
	ext := filepath.Ext(dst)
	name := strings.TrimSuffix(filepath.Base(dst), ext)
	dir := filepath.Dir(dst)

	counter := 1
	for {
		if _, err := os.Stat(finalDst); os.IsNotExist(err) {
			break
		}
		finalDst = filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, counter, ext))
		counter++
	}

	return os.Rename(src, finalDst)
}

// --- Spending Logic ---

type SpendingTx struct {
	Date        time.Time
	Amount      float64
	Source      string // "CIBC", "TD", "Sheet"
	Description string
}

func handleSpending() error {
	var bankTxs []SpendingTx
	var cibcFiles, tdFiles []string
	var sheetFile string

	entries, err := os.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	foundSheet := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "cibc") && strings.HasSuffix(name, ".json") {
			cibcFiles = append(cibcFiles, name)
		} else if strings.HasPrefix(name, "td") && strings.HasSuffix(name, ".csv") {
			tdFiles = append(tdFiles, name)
		} else if name == "sheet.csv" {
			sheetFile = name
			foundSheet = true
		}
	}

	if !foundSheet {
		fmt.Println("Warning: 'sheet.csv' not found. Terminating.")
		return nil
	}

	// 1. Process CIBC JSON files
	for _, file := range cibcFiles {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		txs, err := cibc.Read(f)
		f.Close()

		if err != nil {
			fmt.Printf("Skipping %s: %v\n", file, err)
			continue
		}

		for _, t := range txs {
			bankTxs = append(bankTxs, SpendingTx{Date: t.Date, Amount: t.Amount, Description: t.Description, Source: "CIBC"})
		}
	}

	// 2. Process TD CSV files
	for _, file := range tdFiles {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		txs, err := td.Read(f)
		f.Close()

		if err != nil {
			fmt.Printf("Skipping %s: %v\n", file, err)
			continue
		}

		for _, t := range txs {
			bankTxs = append(bankTxs, SpendingTx{Date: t.Date, Amount: t.Amount, Description: t.Description, Source: "TD"})
		}
	}

	// 3. Read sheet.csv
	sheetTxs, err := readSheet(sheetFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error reading sheet.csv: %w", err)
	}

	// 4. Compare transactions
	var addedTxs, updatedTxs, missingTxs []SpendingTx

	sheetMap := make(map[string][]SpendingTx)
	for _, t := range sheetTxs {
		key := fmt.Sprintf("%s|%.2f", t.Date.Format("2006-01-02"), t.Amount)
		sheetMap[key] = append(sheetMap[key], t)
	}

	for _, bankTx := range bankTxs {
		key := fmt.Sprintf("%s|%.2f", bankTx.Date.Format("2006-01-02"), bankTx.Amount)
		if txs, ok := sheetMap[key]; ok && len(txs) > 0 {
			updatedTxs = append(updatedTxs, bankTx)
			sheetMap[key] = txs[1:]
		} else {
			addedTxs = append(addedTxs, bankTx)
		}
	}

	for _, txs := range sheetMap {
		missingTxs = append(missingTxs, txs...)
	}

	// 5. Display results
	if len(addedTxs) > 0 {
		fmt.Println("\n--- New Transactions (Added) ---")
		for _, tx := range addedTxs {
			fmt.Printf("Date: %s, Amount: %.2f, Description: %s, Source: %s\n", tx.Date.Format("2006-01-02"), tx.Amount, tx.Description, tx.Source)
		}
	}

	if len(updatedTxs) > 0 {
		fmt.Println("\n--- Existing Transactions (Description Updated) ---")
		for _, tx := range updatedTxs {
			fmt.Printf("Date: %s, Amount: %.2f, Description: %s, Source: %s\n", tx.Date.Format("2006-01-02"), tx.Amount, tx.Description, tx.Source)
		}
	}

	if len(missingTxs) > 0 {
		fmt.Println("\n--- Missing Transactions (Warning) ---")
		for _, tx := range missingTxs {
			fmt.Printf("Date: %s, Amount: %.2f, Description: %s\n", tx.Date.Format("2006-01-02"), tx.Amount, tx.Description)
		}
	}

	// 6. Display full transaction list
	fullTxs := append(sheetTxs, addedTxs...)
	fmt.Println("\n--- Full Transaction List ---")
	w := csv.NewWriter(os.Stdout)
	w.Write([]string{"Date", "Amount", "Description", "Source"})
	for _, tx := range fullTxs {
		w.Write([]string{
			tx.Date.Format("2006-01-02"),
			fmt.Sprintf("%.2f", tx.Amount),
			tx.Description,
			tx.Source,
		})
	}
	w.Flush()

	// 7. Move files to deleted
	if err := os.MkdirAll("deleted", 0755); err != nil {
		return err
	}

	for _, file := range cibcFiles {
		moveFile(file, filepath.Join("deleted", file))
	}
	for _, file := range tdFiles {
		moveFile(file, filepath.Join("deleted", file))
	}

	return nil
}

func readSheet(filename string) ([]SpendingTx, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	var result []SpendingTx
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}

		dateStr := row[0]
		amountStr := row[1]
		var description string
		if len(row) > 2 {
			description = row[2]
		}

		formats := []string{"2006-01-02", "01/02/2006", "1/2/2006", "2006/01/02"}
		var parsedDate time.Time
		var dErr error
		for _, layout := range formats {
			parsedDate, dErr = time.Parse(layout, dateStr)
			if dErr == nil {
				break
			}
		}
		if dErr != nil {
			continue
		}

		amountStr = strings.ReplaceAll(amountStr, "$", "")
		amountStr = strings.ReplaceAll(amountStr, ",", "")
		amount, aErr := strconv.ParseFloat(amountStr, 64)
		if aErr != nil {
			continue
		}

		result = append(result, SpendingTx{Date: parsedDate, Amount: amount, Description: description, Source: "Sheet"})
	}
	return result, nil
}
