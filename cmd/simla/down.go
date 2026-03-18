package simla

import (
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down [service-name]",
	Short: "Stop running Lambda containers",
	Long: `Stop one or all running Lambda containers.

Without arguments, stops all running services.
With a service name argument, stops only that service.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sched := scheduler.NewScheduler(cfg, svcRegistry, logger.WithField("component", "scheduler"))

		if len(args) == 1 {
			serviceName := args[0]
			logger.Infof("stopping service %s", serviceName)
			if err := sched.StopService(ctx, serviceName); err != nil {
				logger.WithError(err).Fatalf("failed to stop service %s", serviceName)
			}
			logger.Infof("service %s stopped", serviceName)
			return
		}

		logger.Info("stopping all services")
		if err := sched.StopAll(ctx); err != nil {
			logger.WithError(err).Fatal("failed to stop all services")
		}
		logger.Info("all services stopped")
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
}
