package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	TxnPat     = regexp.MustCompile(`(\d{1,2}/\d{1,2})\s*&?\s*(.*)   \s+((-\s*)?[0-9,]*\.\d+)`)
	SectionPat = regexp.MustCompile(`^\s*(PAYMENTS AND OTHER CREDITS|PURCHASE|FEES CHARGED)`)
)

type rawTxn struct {
	category   string
	date       string
	descriptor string
	amount     string
}

func formatCents(amt int64) string {
	signum := '+'
	if amt < 0 {
		signum = '-'
		amt = -amt
	}
	return fmt.Sprintf("%c%d.%02d", signum, amt/100, amt%100)
}

func processOne(path string) {
	cmd := exec.Command("gs", "-sDEVICE=txtwrite", "-o", "-", path)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("open pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal("starting gs(%q): %v", path, err)
	}

	var raw []rawTxn
	var section string

	r := bufio.NewReader(stdout)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read: %v", err)
		}
		sectMatch := SectionPat.FindSubmatch(line)
		if sectMatch != nil {
			section = string(sectMatch[1])
		}
		matches := TxnPat.FindSubmatch(line)
		if matches == nil {
			continue
		}
		// fmt.Printf("%s,%s,%s\n", matches[1], bytes.TrimRight(matches[2], " "), matches[3])
		raw = append(raw, rawTxn{
			section,
			string(matches[1]),
			string(bytes.TrimRight(matches[2], " ")),
			string(matches[3]),
		})
	}

	if err := cmd.Wait(); err != nil {
		log.Fatal("converting to text: %q: %v", path, err)
	}

	totals := make(map[string]int64)
	for _, txn := range raw {
		amt := txn.amount
		amt = strings.Replace(amt, ",", "", -1)
		amt = strings.Replace(amt, ".", "", -1)
		cents, err := strconv.ParseInt(amt, 10, 64)
		if err != nil {
			fmt.Printf("parse(%s): %v", amt, err)
			continue
		}
		totals[txn.category] += cents
	}
	fmt.Printf("TOTALS -- %s\n", path)
	for cat, amt := range totals {
		fmt.Printf("%30s %s\n", cat, formatCents(amt))
	}
}

func main() {
	for _, path := range os.Args[1:] {
		processOne(path)
	}
}
