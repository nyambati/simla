package simla

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered Lambda services",
	Long:  `Print a table of all services known to the registry with their status, port, container ID, and health.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		services := svcRegistry.ListServices(ctx)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTATUS\tPORT\tCONTAINER ID\tHEALTHY")
		fmt.Fprintln(w, "----\t------\t----\t------------\t-------")

		for _, svc := range services {
			containerID := svc.ID
			if len(containerID) > 12 {
				containerID = containerID[:12]
			}
			if containerID == "" {
				containerID = "-"
			}

			status := string(svc.Status)
			if status == "" {
				status = "unknown"
			}

			healthy := "false"
			if svc.Healthy {
				healthy = "true"
			}

			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
				svc.Name,
				status,
				svc.Port,
				containerID,
				healthy,
			)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
