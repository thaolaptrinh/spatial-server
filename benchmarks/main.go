package main

import (
	"flag"
	"log"
	"time"

	"github.com/thaolaptrinh/spatial-server/benchmarks/framework"
)

func main() {
	scenario := flag.String("scenario", "light", "light|medium|heavy|burst|zone_transfer|stability")
	addr := flag.String("addr", "localhost:8080", "Gateway address")
	flag.Parse()

	r := framework.NewReport(*scenario)
	var err error

	switch *scenario {
	case "light":
		err = runBenchmark(*addr, r, 100, 1*time.Minute)
	case "medium":
		err = runBenchmark(*addr, r, 1000, 5*time.Minute)
	case "heavy":
		err = runBenchmark(*addr, r, 5000, 10*time.Minute)
	default:
		log.Printf("scenario %s: stub implementation", *scenario)
	}

	if err != nil {
		log.Printf("scenario %s: %v", *scenario, err)
		r.Pass = false
	} else {
		r.Pass = true
	}
	r.Write()
	r.PrintSummary()
}

func runBenchmark(addr string, r *framework.Report, clients int, duration time.Duration) error {
	return nil
}
