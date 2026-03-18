package simla

import (
	"fmt"
	"os"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/nyambati/simla/internal/runtime"
	"github.com/spf13/cobra"
)

var logsFollow bool

var logsCmd = &cobra.Command{
	Use:   "logs <service-name>",
	Short: "Show logs for a Lambda service container",
	Long: `Stream or print the stdout/stderr output of a Lambda service container.

Use --follow / -f to keep the stream open until the container stops or Ctrl+C is pressed.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		serviceName := args[0]

		svc, exists := svcRegistry.GetService(ctx, serviceName)
		if !exists {
			logger.Fatalf("service %q not found in registry", serviceName)
		}

		if svc.ID == "" {
			logger.Fatalf("service %q has no associated container (has it been started?)", serviceName)
		}

		rt, err := runtime.NewRuntime(svcRegistry, logger.WithField("component", "runtime"))
		if err != nil {
			logger.WithError(err).Fatal("failed to create runtime client")
		}

		logCtx := ctx
		if logsFollow {
			// Use the signal-aware context so Ctrl+C closes the stream.
			logCtx = ctx
		}

		reader, err := rt.GetLogs(logCtx, svc.ID, logsFollow)
		if err != nil {
			logger.WithError(err).Fatalf("failed to get logs for service %s", serviceName)
		}
		defer reader.Close()

		// Docker multiplexes stdout and stderr into a single stream with an
		// 8-byte header per frame. stdcopy.StdCopy demultiplexes it.
		_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading log stream: %v\n", err)
		}
	},
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output (stream until container stops)")
	rootCmd.AddCommand(logsCmd)
}
