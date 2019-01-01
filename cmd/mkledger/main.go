package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"time"

	"nelhage.com/ccstatements/money"
)

type pattern struct {
	pat     string
	date    string
	account string
}

var patterns = []pattern{}

type compiledPattern struct {
	pat     *regexp.Regexp
	date    time.Time
	account string
}

var compiledPatterns []compiledPattern

func init() {
	for _, pat := range patterns {
		comp := compiledPattern{
			pat:     regexp.MustCompile(pat.pat),
			account: pat.account,
		}

		if pat.date != "" {
			var err error
			comp.date, err = time.Parse("2006-01-02", pat.date)
			if err != nil {
				log.Fatalf("pattern: parse: %q: %v", pat.date, err)
			}
		}
		compiledPatterns = append(compiledPatterns, comp)
	}
}

func categorize(date time.Time, descriptor string) string {
	if descriptor == "AUTOMATIC PAYMENT - THANK YOU" {
		return "Assets:Checking"
	}
	for _, pat := range compiledPatterns {
		if !pat.pat.MatchString(descriptor) {
			continue
		}
		if pat.date.IsZero() || pat.date == date {
			return pat.account
		}
	}
	return "Expenses:Unknown"
}

func processOne(path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	read := csv.NewReader(fh)

	fmt.Printf("; -*- mode: ledger -*-\n\n")

	for {
		fields, err := read.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// category := fields[0]
		last4 := fields[1]
		date, err := time.Parse("2006-01-02", fields[2])
		if err != nil {
			return err
		}
		descriptor := fields[3]
		amount, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return err
		}

		account := categorize(date, descriptor)
		fmt.Printf("%s %s\n", date.Format("2006-01-02"), descriptor)
		fmt.Printf("  Liabilities:%s  $ %s\n", last4, money.FormatCents(-amount))
		fmt.Printf("  %s\n", account)
		fmt.Println()
	}

	return nil
}

func main() {
	flag.Parse()

	for _, path := range flag.Args() {
		if err := processOne(path); err != nil {
			log.Fatal("%s: %v", path, err)
		}
	}
}
