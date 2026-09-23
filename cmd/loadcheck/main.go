// Command loadcheck measures actual HTTP masking or mask+demask round trips.
// It prints only aggregate measurements, never API keys or payload values.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type outcome struct {
	ms        float64
	status    int
	roundtrip bool
	requests  int
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8080", "HTTP base URL")
	systems := flag.String("systems", ".runtime/adaptive/systems.json", "API key file")
	duration := flag.Duration("duration", 10*time.Second, "measurement window")
	concurrency := flag.Int("concurrency", 32, "parallel clients")
	size := flag.Int("chars", 250, "approximate Unicode code points per payload")
	roundtrip := flag.Bool("roundtrip", false, "mask then demask and check exact restoration")
	flag.Parse()
	if *concurrency < 1 || *size < 1 {
		panic("positive concurrency/chars required")
	}
	data, err := os.ReadFile(*systems)
	if err != nil {
		panic(err)
	}
	var configs []struct {
		APIKey string `json:"api_key"`
	}
	if err = json.Unmarshal(data, &configs); err != nil || len(configs) == 0 {
		panic("invalid systems file")
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, MaxIdleConns: 2 * *concurrency, MaxIdleConnsPerHost: 2 * *concurrency, MaxConnsPerHost: 2 * *concurrency}}
	defer client.CloseIdleConnections()
	seed := "Меня зовут Иванов Пётр Сергеевич. Адрес: Москва, улица Тестовая, дом 7, квартира 12. Email example@example.com. Позвоните +7 999 123-45-67. Передайте Ольге Савельевой сообщение о встрече завтра. "
	text := strings.Repeat(seed, (*size+utf8.RuneCountInString(seed)-1)/utf8.RuneCountInString(seed))
	text = string([]rune(text)[:*size])
	request := func(payload, id string) (string, int) {
		body, _ := json.Marshal(map[string]string{"payload": payload, "payload_id": id})
		req, _ := http.NewRequest("POST", strings.TrimRight(*base, "/")+"/process", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", configs[0].APIKey)
		resp, err := client.Do(req)
		if err != nil {
			return "", 0
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", 0
		}
		if resp.StatusCode != 200 {
			return "", resp.StatusCode
		}
		var out struct {
			Result string `json:"result"`
		}
		if json.Unmarshal(raw, &out) != nil {
			return "", -1
		}
		return out.Result, 200
	}
	run := fmt.Sprintf("load-%d", time.Now().UnixNano())
	start := time.Now()
	deadline := start.Add(*duration)
	results := make(chan []outcome, *concurrency)
	var wg sync.WaitGroup
	for worker := 0; worker < *concurrency; worker++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			local := []outcome{}
			for i := 0; time.Now().Before(deadline); i++ {
				id := fmt.Sprintf("%s-%d-%d", run, w, i)
				t := time.Now()
				masked, status := request(text, id)
				o := outcome{status: status, requests: 1}
				if *roundtrip && status == 200 {
					restored, s := request(masked, id)
					o.requests++
					o.status = s
					o.roundtrip = s == 200 && restored == text
					if s == 200 && !o.roundtrip {
						o.status = -2
					}
				}
				o.ms = float64(time.Since(t)) / float64(time.Millisecond)
				local = append(local, o)
			}
			results <- local
		}(worker)
	}
	wg.Wait()
	close(results)
	elapsed := time.Since(start).Seconds()
	latencies := []float64{}
	statuses := map[int]int{}
	total, success, requests, roundtrips := 0, 0, 0, 0
	for batch := range results {
		for _, o := range batch {
			total++
			requests += o.requests
			statuses[o.status]++
			if o.status == 200 {
				success++
				latencies = append(latencies, o.ms)
			}
			if o.roundtrip {
				roundtrips++
			}
		}
	}
	sort.Float64s(latencies)
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return latencies[int(float64(len(latencies)-1)*p)]
	}
	mode := "mask"
	if *roundtrip {
		mode = "mask_then_demask"
	}
	out := map[string]any{"mode": mode, "chars": *size, "concurrency": *concurrency, "elapsed_seconds": elapsed, "transactions": total, "successful_transactions": success, "success_transactions_per_second": float64(success) / elapsed, "http_requests_per_second": float64(requests) / elapsed, "error_rate": float64(total-success) / float64(max(1, total)), "statuses": statuses, "p50_ms": percentile(.5), "p95_ms": percentile(.95), "p99_ms": percentile(.99), "exact_round_trips": roundtrips, "latency_population": "successful transactions; in-flight requests drained after measurement window"}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
