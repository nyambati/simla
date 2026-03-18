package simla

import (
	"context"

	"github.com/nyambati/simla/internal/gateway"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/nyambati/simla/internal/watcher"
	"github.com/spf13/cobra"
)

var watchMode bool

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start simla server",
	Long:  `Start the simla local Lambda development server.`,
	Run: func(cmd *cobra.Command, args []string) {
		sched := scheduler.NewScheduler(cfg, svcRegistry, logger.WithField("component", "scheduler"))
		gw := gateway.NewAPIGateway(cfg, svcRegistry, logger)

		if watchMode {
			w := watcher.New(cfg, sched, logger.WithField("component", "watcher"), 0)
			go func() {
				if err := w.Start(ctx); err != nil {
					logger.WithError(err).Error("watcher exited with error")
				}
			}()
			logger.Info("hot reload enabled — watching service code paths")
		}

		if err := gw.Start(ctx); err != nil {
			logger.WithError(err).Error("gateway exited with error")
		}

		// ctx is now done (signal received). Stop all running Lambda containers.
		stopCtx()
		logger.Info("stopping all services")
		if err := sched.StopAll(context.Background()); err != nil {
			logger.WithError(err).Error("error stopping services")
		}
	},
}

func init() {
	upCmd.Flags().BoolVarP(&watchMode, "watch", "w", false, "Enable hot reload: restart services when their code changes")
	rootCmd.AddCommand(upCmd)
}
