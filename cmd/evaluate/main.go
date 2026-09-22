// Command evaluate runs the rule engine over the built-in evaluation dataset
// and prints precision/recall/F1, typed metrics, false positive rate and
// latency.
package main

import (
	"fmt"
	"os"

	"github.com/kryneuse/alpha_proxy/internal/engine"
	"github.com/kryneuse/alpha_proxy/internal/eval"
)

func main() {
	e := engine.New(engine.Options{})
	metrics := eval.Evaluate(e, eval.Dataset())
	metrics.Print()

	// Exit non-zero if typed span recall is below a reasonable bar.
	if metrics.TypedSpanRecall < 0.5 {
		fmt.Fprintln(os.Stderr, "WARNING: typed span recall below 0.5")
		os.Exit(1)
	}
}
