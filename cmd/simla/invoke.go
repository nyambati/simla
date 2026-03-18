package simla

import (
	"fmt"
	"os"

	"github.com/nyambati/simla/internal/scheduler"
	"github.com/spf13/cobra"
)

var invokePayload string
var invokeFile string

var invokeCmd = &cobra.Command{
	Use:   "invoke <service-name>",
	Short: "Directly invoke a Lambda service",
	Long: `Invoke a Lambda service by name, bypassing the HTTP API gateway.

The payload can be supplied inline with --payload or read from a file with --file.
If neither flag is provided, an empty JSON object {} is sent.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := args[0]

		var payload []byte

		switch {
		case invokeFile != "":
			data, err := os.ReadFile(invokeFile)
			if err != nil {
				logger.WithError(err).Fatalf("failed to read payload file %s", invokeFile)
			}
			payload = data
		case invokePayload != "":
			payload = []byte(invokePayload)
		default:
			payload = []byte("{}")
		}

		// Put service name in context so health checks and routing work correctly.
		invokeCtx := cmd.Context()

		sched := scheduler.NewScheduler(cfg, svcRegistry, logger.WithField("component", "scheduler"))
		response, err := sched.Invoke(invokeCtx, serviceName, payload)
		if err != nil {
			logger.WithError(err).Fatalf("invocation failed for service %s", serviceName)
		}

		fmt.Println(string(response))
	},
}

func init() {
	invokeCmd.Flags().StringVarP(&invokePayload, "payload", "p", "", "JSON payload to send to the Lambda")
	invokeCmd.Flags().StringVarP(&invokeFile, "file", "f", "", "Path to a JSON file to use as the payload")
	rootCmd.AddCommand(invokeCmd)
}
