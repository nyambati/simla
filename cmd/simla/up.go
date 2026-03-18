/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/

package simla

import (
	"context"

	"github.com/nyambati/simla/internal/gateway"
	"github.com/nyambati/simla/internal/scheduler"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "start simla server",
	Long:  `Start simla server`,
	Run: func(cmd *cobra.Command, args []string) {
		sched := scheduler.NewScheduler(cfg, svcRegistry, logger.WithField("component", "scheduler"))
		gw := gateway.NewAPIGateway(cfg, svcRegistry, logger)
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
	rootCmd.AddCommand(upCmd)
}
