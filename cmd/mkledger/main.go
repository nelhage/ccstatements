package main

import (
	"encoding/csv"
	"errors"
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

type compiledPattern struct {
	pat     *regexp.Regexp
	date    time.Time
	account string
}

var paymentPat = regexp.MustCompile("(AUTOMATIC PAYMENT - THANK YOU)|(PAYMENT THANK YOU)")

func categorize(pats []compiledPattern, date time.Time, descriptor string) string {
	if paymentPat.MatchString(descriptor) {
		return "Assets:Checking"
	}
	for _, pat := range pats {
		if !pat.pat.MatchString(descriptor) {
			continue
		}
		if pat.date.IsZero() || pat.date == date {
			return pat.account
		}
	}
	return "Expenses:Unknown"
}

const (
	CatRedemptions = "PURCHASES AND REDEMPTIONS"
)

func processOne(pats []compiledPattern, path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	read := csv.NewReader(fh)

	for {
		fields, err := read.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		category := fields[0]
		if category == CatRedemptions {
			continue
		}
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

		account := categorize(pats, date, descriptor)
		fmt.Printf("%s %s\n", date.Format("2006-01-02"), descriptor)
		fmt.Printf("  Liabilities:%s  $ %s\n", last4, money.FormatCents(-amount))
		fmt.Printf("  %s\n", account)
		fmt.Println()
	}

	return nil
}

func loadPatterns(path string) ([]compiledPattern, error) {
	var patterns []compiledPattern

	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	read := csv.NewReader(fh)

	for {
		fields, err := read.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if len(fields) < 3 {
			return nil, errors.New("not enough fields")
		}

		reText := fields[0]
		date := fields[1]
		account := fields[2]

		re, err := regexp.Compile(reText)
		if err != nil {
			return nil, err
		}

		comp := compiledPattern{
			pat:     re,
			account: account,
		}

		if date != "" {
			var err error
			comp.date, err = time.Parse("2006-01-02", date)
			if err != nil {
				return nil, fmt.Errorf("pattern: parse: %q: %v", date, err)
			}
		}
		patterns = append(patterns, comp)
	}

	return patterns, nil
}

func main() {
	var (
		patCsv = flag.String("patterns", "", "Path to an attribution-pattern CSV")
	)
	flag.Parse()

	var pats []compiledPattern
	if *patCsv != "" {
		var err error
		pats, err = loadPatterns(*patCsv)
		if err != nil {
			log.Fatalf("parsing patterns: %v", err)
		}
	}

	fmt.Printf("; -*- mode: ledger -*-\n\n")

	for _, path := range flag.Args() {
		if err := processOne(pats, path); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
	}
}
