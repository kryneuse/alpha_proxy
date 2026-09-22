// Command evaluate runs the rule engine over the built-in evaluation dataset
// and prints precision/recall/F1, typed metrics, false positive rate and
// latency.
package main

import (
	"fmt"
	"os"

	"github.com/alpha-proxy/rule-engine/internal/engine"
	"github.com/alpha-proxy/rule-engine/internal/eval"
)

func main() {
	e := engine.New(engine.Options{})
	metrics := eval.Evaluate(e, eval.Dataset())
	metrics.Print()

	// Exit non-zero if recall is below a reasonable bar.
	if metrics.Recall < 0.5 {
		fmt.Fprintln(os.Stderr, "WARNING: recall below 0.5")
		os.Exit(1)
	}
}
