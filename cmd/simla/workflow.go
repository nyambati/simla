package simla

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/nyambati/simla/internal/scheduler"
	"github.com/nyambati/simla/internal/workflow"
	"github.com/spf13/cobra"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and run Step Functions workflows",
	Long:  `Commands for listing and executing Step Functions-style workflows defined in .simla.yaml.`,
}

// ---------------------------------------------------------------------------
// workflow list
// ---------------------------------------------------------------------------

var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workflows defined in .simla.yaml",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if len(cfg.Workflows) == 0 {
			fmt.Println("No workflows defined.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTARTS AT\tSTATES\tCOMMENT")
		fmt.Fprintln(w, "----\t---------\t------\t-------")

		for name, sm := range cfg.Workflows {
			comment := sm.Comment
			if comment == "" {
				comment = "-"
			}
			displayName := sm.Name
			if displayName == "" {
				displayName = name
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				displayName,
				sm.StartAt,
				len(sm.States),
				comment,
			)
		}
		w.Flush()
	},
}

// ---------------------------------------------------------------------------
// workflow run
// ---------------------------------------------------------------------------

var workflowRunPayload string
var workflowRunFile string
var workflowRunPretty bool

var workflowRunCmd = &cobra.Command{
	Use:   "run <workflow-name>",
	Short: "Execute a workflow",
	Long: `Run a Step Functions workflow by name.

The input can be supplied inline with --input or read from a file with --file.
If neither flag is provided, an empty JSON object {} is used.

Example:
  simla workflow run order-pipeline --input '{"orderId":"123"}'
  simla workflow run order-pipeline --file ./event.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		workflowName := args[0]

		var input []byte
		switch {
		case workflowRunFile != "":
			data, err := os.ReadFile(workflowRunFile)
			if err != nil {
				logger.WithError(err).Fatalf("failed to read input file %s", workflowRunFile)
			}
			input = data
		case workflowRunPayload != "":
			input = []byte(workflowRunPayload)
		default:
			input = []byte("{}")
		}

		// Validate that input is well-formed JSON before sending it into the engine.
		if !json.Valid(input) {
			logger.Fatalf("input is not valid JSON")
		}

		sched := scheduler.NewScheduler(cfg, svcRegistry, logger.WithField("component", "scheduler"))
		executor := workflow.NewExecutor(cfg, sched, logger.WithField("component", "workflow"))

		runCtx := cmd.Context()
		output, err := executor.Execute(runCtx, workflowName, input)
		if err != nil {
			logger.WithError(err).Fatalf("workflow %s failed", workflowName)
		}

		if workflowRunPretty {
			var v any
			if err := json.Unmarshal(output, &v); err == nil {
				pretty, err := json.MarshalIndent(v, "", "  ")
				if err == nil {
					fmt.Println(string(pretty))
					return
				}
			}
		}

		fmt.Println(string(output))
	},
}

func init() {
	workflowRunCmd.Flags().StringVarP(&workflowRunPayload, "input", "i", "", "JSON input to pass to the workflow")
	workflowRunCmd.Flags().StringVarP(&workflowRunFile, "file", "f", "", "Path to a JSON file to use as the workflow input")
	workflowRunCmd.Flags().BoolVar(&workflowRunPretty, "pretty", false, "Pretty-print the JSON output")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	rootCmd.AddCommand(workflowCmd)
}
