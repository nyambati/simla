package simla

import (
	"context"

	"github.com/nyambati/simla/internal/gateway"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/nyambati/simla/internal/trigger"
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

		// Start all configured triggers in the background.
		startTriggers(sched)

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

// startTriggers iterates all services in the config, constructs a trigger
// Source for each Trigger definition, and runs them as background goroutines.
func startTriggers(sched scheduler.SchedulerInterface) {
	for serviceName, svc := range cfg.Services {
		for _, trig := range svc.Triggers {
			src, err := trigger.New(
				trig,
				serviceName,
				sched,
				logger.WithFields(map[string]interface{}{
					"component": "trigger",
					"service":   serviceName,
					"type":      string(trig.Type),
				}),
			)
			if err != nil {
				logger.WithError(err).Errorf("failed to configure trigger for service %s", serviceName)
				continue
			}

			go func(s trigger.Source, name string, trigType string) {
				logger.Infof("starting %s trigger for service %s", trigType, name)
				if err := s.Start(ctx); err != nil {
					logger.WithError(err).Errorf("%s trigger failed for service %s", trigType, name)
				}
			}(src, serviceName, string(trig.Type))
		}
	}
}

func init() {
	upCmd.Flags().BoolVarP(&watchMode, "watch", "w", false, "Enable hot reload: restart services when their code changes")
	rootCmd.AddCommand(upCmd)
}
