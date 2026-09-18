// Command parity replays recorded requests against two services and diffs the JSON responses
// field by field. This is the migration runbook's parity harness (ticket 9).
//
//	go run ./cmd/parity --a https://api-server --b http://localhost:3000 \
//	   --cases cmd/parity/testdata/sample.jsonl --ignore txnDate,createdAt,updatedAt,timestamp
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Case is one recorded request.
type Case struct {
	Name    string            `json:"name"`
	Tenant  string            `json:"tenant"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func main() {
	aBase := flag.String("a", "", "baseline base URL (the Node service)")
	bBase := flag.String("b", "", "candidate base URL (tr-bah-service)")
	casesPath := flag.String("cases", "", "JSONL file of recorded requests")
	ignoreList := flag.String("ignore", "txnDate,createdAt,updatedAt,timestamp", "comma-separated JSON paths or leaf names to ignore")
	timeout := flag.Duration("timeout", 30*time.Second, "per-request timeout")
	flag.Parse()

	if *aBase == "" || *bBase == "" || *casesPath == "" {
		fmt.Fprintln(os.Stderr, "parity: --a, --b and --cases are required")
		os.Exit(2)
	}
	ignore := map[string]bool{}
	for _, k := range strings.Split(*ignoreList, ",") {
		if k = strings.TrimSpace(k); k != "" {
			ignore[k] = true
		}
	}
	f, err := os.Open(*casesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parity:", err)
		os.Exit(2)
	}
	defer f.Close()

	client := &http.Client{Timeout: *timeout}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	total, failed := 0, 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c Case
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			fmt.Printf("SKIP  (unparsable case) %v\n", err)
			continue
		}
		total++
		name := c.Name
		if name == "" {
			name = c.Method + " " + c.Path
		}
		aStatus, aBody, aErr := call(client, *aBase, c)
		bStatus, bBody, bErr := call(client, *bBase, c)
		if aErr != nil || bErr != nil {
			failed++
			fmt.Printf("ERROR %s\n  a: %v\n  b: %v\n", name, aErr, bErr)
			continue
		}
		var diffs []Diff
		if aStatus != bStatus {
			diffs = append(diffs, Diff{Path: "$status", A: aStatus, B: bStatus, Note: "status mismatch"})
		}
		diffs = append(diffs, CompareJSON(aBody, bBody, ignore)...)
		if len(diffs) == 0 {
			fmt.Printf("PASS  %s\n", name)
			continue
		}
		failed++
		fmt.Printf("DIFF  %s (%d)\n", name, len(diffs))
		for _, d := range diffs {
			fmt.Printf("        %s\n", d)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "parity:", err)
		os.Exit(2)
	}
	fmt.Printf("\n%d cases, %d clean, %d with differences\n", total, total-failed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func call(client *http.Client, base string, c Case) (int, any, error) {
	method := c.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if len(c.Body) > 0 && string(c.Body) != "null" {
		body = bytes.NewReader(c.Body)
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+c.Path, body)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	if c.Tenant != "" {
		req.Header.Set("x-tenant-id", c.Tenant)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	var decoded any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return resp.StatusCode, string(raw), nil
		}
	}
	return resp.StatusCode, decoded, nil
}
