/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/

package simla

import (
	"github.com/nyambati/simla/internal/gateway"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "start simla server",
	Long:  `Start simla server`,
	Run: func(cmd *cobra.Command, args []string) {
		gateway := gateway.NewAPIGateway(cfg, svcRegistry, logger)
		if err := gateway.Start(ctx); err != nil {
			logger.WithError(err).Fatal("Failed to start api gateway")
		}
	},
}

func init() {
	rootCmd.AddCommand(upCmd)
}
