package simla

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/nyambati/simla/internal/scheduler"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show per-service invocation metrics",
	Long: `Display a summary table of Lambda invocation metrics collected since simla up was last started.

Columns:
  SERVICE     Name of the Lambda service
  INVOCATIONS Total number of invocations recorded
  ERRORS      Number of invocations that returned an error
  ERROR RATE  Fraction of invocations that errored (0.00–1.00)
  AVG LATENCY Mean invocation round-trip time
  LAST INVOKED Wall-clock time of the most recent invocation`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		all := scheduler.GlobalMetrics.All()

		if len(all) == 0 {
			fmt.Println("No invocation data recorded yet. Run simla up and invoke some services first.")
			return
		}

		// Sort service names for stable output.
		names := make([]string, 0, len(all))
		for name := range all {
			names = append(names, name)
		}
		sort.Strings(names)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "SERVICE\tINVOCATIONS\tERRORS\tERROR RATE\tAVG LATENCY\tLAST INVOKED")
		fmt.Fprintln(w, "-------\t-----------\t------\t----------\t-----------\t------------")

		for _, name := range names {
			m := all[name]

			lastInvoked := "-"
			if !m.LastInvoked.IsZero() {
				lastInvoked = m.LastInvoked.Format(time.RFC3339)
			}

			fmt.Fprintf(w, "%s\t%d\t%d\t%.2f\t%s\t%s\n",
				name,
				m.Invocations,
				m.Errors,
				m.ErrorRate(),
				m.AvgLatency().Round(time.Millisecond),
				lastInvoked,
			)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
