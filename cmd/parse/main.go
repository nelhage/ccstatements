package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nelhage.com/ccstatements/money"
)

var (
	TxnPat     = regexp.MustCompile(`(\d{1,2}/\d{1,2})\s*&?\s*(.*)   \s+((-\s*)?[0-9,]*\.\d+)`)
	SectionPat = regexp.MustCompile(`^\s*(PAYMENTS AND OTHER CREDITS|PURCHASE|FEES CHARGED|PURCHASES AND REDEMPTIONS)\s`)
	HeaderPat  = regexp.MustCompile(`^\s*((?:\S|\s\S)+)  [ \t]+((?:[+-]?\s*[$][0-9,]+\.\d{2})|(?:\d{2}/\d{2}/\d{2} - \d{2}/\d{2}/\d{2}))`)
	DatePat    = regexp.MustCompile(`(\d{2}/\d{2}/\d{2}) - (\d{2}/\d{2}/\d{2})`)
	PANPat     = regexp.MustCompile(`Account Number:\s*(\d{4} \d{4} \d{4} (\d{4}))`)
)

type rawFile struct {
	txns    []rawTxn
	headers map[string]string
	last4   string
}

type rawTxn struct {
	category   string
	date       string
	descriptor string
	amount     string
}

type Transaction struct {
	Category   string
	Date       time.Time
	Descriptor string
	Amount     int64
}

type Statement struct {
	StartDate, EndDate time.Time
	Last4              string
	Transactions       []Transaction
}

func parseAmount(amount string) (int64, error) {
	amount = strings.Replace(amount, ",", "", -1)
	amount = strings.Replace(amount, " ", "", -1)
	amount = strings.Replace(amount, ".", "", 1)
	amount = strings.Replace(amount, "$", "", 1)
	return strconv.ParseInt(amount, 10, 64)
}

func interpret(raw *rawFile) (*Statement, error) {
	dateHdr, ok := raw.headers["Opening/Closing Date"]
	if !ok {
		return nil, fmt.Errorf("Missing date header. Got: %#v", raw.headers)
	}
	stmt := &Statement{Last4: raw.last4}

	dateMatch := DatePat.FindStringSubmatch(dateHdr)
	if dateMatch != nil {
		stmt.StartDate, _ = time.Parse("01/02/06", dateMatch[1])
		stmt.EndDate, _ = time.Parse("01/02/06", dateMatch[2])
	}
	if stmt.StartDate.IsZero() || stmt.EndDate.IsZero() {
		return nil, fmt.Errorf("Can't parse date header: %s", dateHdr)
	}

	minDate := stmt.StartDate.AddDate(-1, 0, 0)
	maxDate := stmt.EndDate.AddDate(0, 0, 1)
	for _, rawTxn := range raw.txns {
		date, err := time.Parse("01/02", rawTxn.date)
		if err != nil {
			return nil, fmt.Errorf("parse date(%v): %v", rawTxn.date, err)
		}
		date = time.Date(stmt.EndDate.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		if date.After(maxDate) {
			date = date.AddDate(-1, 0, 0)
		}
		if date.After(maxDate) || date.Before(minDate) {
			return nil, fmt.Errorf("date(%s, interpreted as %s) out of range: %s--%s",
				rawTxn.date,
				date.Format("2006-01-02"),
				minDate.Format("2006-01-02"),
				maxDate.Format("2006-01-02"),
			)
		}

		cents, err := parseAmount(rawTxn.amount)

		if err != nil {
			return nil, err
		}
		stmt.Transactions = append(stmt.Transactions, Transaction{
			rawTxn.category,
			date,
			rawTxn.descriptor,
			cents,
		})
	}
	return stmt, nil
}

func writeCsv(path string, stmt *Statement) error {
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer fh.Close()
	write := csv.NewWriter(fh)
	for _, txn := range stmt.Transactions {
		write.Write([]string{
			txn.Category,
			stmt.Last4,
			txn.Date.Format("2006-01-02"),
			txn.Descriptor,
			strconv.FormatInt(txn.Amount, 10),
		})
	}
	write.Flush()
	return write.Error()
}

var (
	debug = flag.Bool("debug", false, "Debug transactions")
)

type expectation struct {
	section, header string
}

var expectations = []expectation{
	{"PAYMENTS AND OTHER CREDITS", "Payment, Credits"},
	{"PURCHASE", "Purchases"},
	{"FEES CHARGED", "Fees Charged"},
}

func processOne(path string) error {
	cmd := exec.Command("gs", "-sDEVICE=txtwrite", "-o", "-", path)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting gs: %v", err)
	}

	raw := rawFile{
		headers: make(map[string]string),
	}
	var section string

	r := bufio.NewReader(stdout)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read: %v", err)
		}
		if bytes.IndexByte(line, '`') > 0 {
			line = bytes.Replace(line, []byte{'`'}, nil, -1)
		}

		if sectMatch := SectionPat.FindSubmatch(line); sectMatch != nil {
			section = string(sectMatch[1])
			continue
		}

		if hdrMatch := HeaderPat.FindSubmatch(line); hdrMatch != nil {
			raw.headers[string(hdrMatch[1])] = string(hdrMatch[2])
			continue
		}
		if panMatch := PANPat.FindSubmatch(line); panMatch != nil {
			raw.last4 = string(panMatch[2])
			continue
		}

		matches := TxnPat.FindSubmatch(line)
		if matches == nil {
			continue
		}
		if section == "" {
			return errors.New("transaction found without a section")
		}
		if *debug {
			fmt.Printf("%s,%s,%s,%s\n",
				section,
				matches[1],
				bytes.TrimRight(matches[2], " "),
				matches[3])
		}
		raw.txns = append(raw.txns, rawTxn{
			section,
			string(matches[1]),
			string(bytes.TrimRight(matches[2], " ")),
			string(matches[3]),
		})
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("converting to text: %v", err)
	}

	totals := make(map[string]int64)
	for _, txn := range raw.txns {
		cents, err := parseAmount(txn.amount)
		if err != nil {
			return fmt.Errorf("parse(%s): %v", txn.amount, err)
		}
		totals[txn.category] += cents
	}

	fmt.Printf("# statement: %s\n", path)
	fmt.Printf("TOTALS\n")
	for cat, amt := range totals {
		fmt.Printf("%30s %s\n", cat, money.FormatCents(amt))
	}
	if *debug {
		fmt.Printf("\nHEADERS\n")
		for k, v := range raw.headers {
			fmt.Printf("%30s %s\n", k, v)
		}
	}

	for _, expect := range expectations {
		hdr, ok := raw.headers[expect.header]
		if !ok {
			return fmt.Errorf("Missing header: %q", expect.header)
		}
		headerAmt, err := parseAmount(hdr)
		if err != nil {
			return fmt.Errorf("Parsing header(%q): %v", expect.header, err)
		}
		if headerAmt != totals[expect.section] {
			return fmt.Errorf("Mismatch: %v(%s) != %v(%s)",
				expect.section, money.FormatCents(totals[expect.section]),
				expect.header, money.FormatCents(headerAmt),
			)
		}
	}

	stmt, err := interpret(&raw)
	if err != nil {
		return err
	}

	if strings.HasSuffix(path, ".pdf") {
		return writeCsv(strings.TrimSuffix(path, "pdf")+"csv", stmt)
	}

	return nil
}

func main() {
	var (
		failFast = flag.Bool("fail-fast", false, "Exit after first parse failure")
	)
	flag.Parse()
	ok := true
	for _, path := range flag.Args() {
		if err := processOne(path); err != nil {
			ok = false
			log.Printf("process(%q): %v", path, err)
			if *failFast {
				break
			}
		}
	}
	if !ok {
		os.Exit(1)
	}
}
