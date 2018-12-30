package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
)

var PAT = regexp.MustCompile(`(\d{1,2}/\d{1,2})\s*&?\s*(.*)   \s+((-\s*)?[0-9,]*\.\d+)`)

func readOne(fh io.Reader) {
	var pos, neg int64

	r := bufio.NewReader(fh)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read: %v", err)
		}
		matches := PAT.FindSubmatch(line)
		if matches == nil {
			continue
		}
		fmt.Printf("%s,%s,%s\n", matches[1], bytes.TrimRight(matches[2], " "), matches[3])

		amt := matches[3]
		amt = bytes.Replace(amt, []byte(","), []byte{}, -1)
		amt = bytes.Replace(amt, []byte("."), []byte{}, -1)
		cents, err := strconv.ParseInt(string(amt), 10, 64)
		if err != nil {
			fmt.Printf("parse(%s): %v", amt, err)
		} else {
			if cents > 0 {
				pos += cents
			} else {
				neg -= cents
			}
		}
	}
	fmt.Printf("TOTAL: +%d.%d -%d.%d\n", pos/100, pos%100, neg/100, neg%100)
}

func main() {
	for _, path := range os.Args {
		fh, e := os.Open(path)
		if e != nil {
			log.Fatalf("open(%q): %v", path, e)
		}
		defer fh.Close()
		readOne(fh)
	}
}

// perl -lane 'print "$1,$2,$3" if m {(\d{1,2}/\d{1,2})\s*&?\s*(.*)   \s+(\d+\.\d+)}' 20180117-statements-4997-.txt  | less
