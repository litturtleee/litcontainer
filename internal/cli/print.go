package cli

import (
	"fmt"
	"litcontainer/internal/container"
	"litcontainer/internal/logger"
	"litcontainer/internal/network"
	"os"
	"strings"
	"text/tabwriter"
)

// printContainersInfo 输出所有容器信息
func printContainersInfo(configs []*container.Config) error {
	// 格式化输出
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPID\tCOMMAND\tSTATE\tSTARTED_AT\tUPDATED_AT")
	for _, config := range configs {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			config.ID[:12],
			config.Name,
			config.Pid,
			strings.Join(config.Command, " "),
			config.State,
			config.CreatedAt,
			config.UpdateAt,
		)
	}
	if err := w.Flush(); err != nil {
		logger.Error("Failed to flush container info, err: %v", err)
		return err
	}
	return nil
}

// printNetworksInfo 输出所有网络信息
func printNetworksInfo(nws []*network.Network) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tIPRANGE\tDRIVER")
	for _, nw := range nws {
		fmt.Fprintf(w, "%s\t%s\t%s\n", nw.Name, nw.IpRange, nw.Driver)
	}
	if err := w.Flush(); err != nil {
		logger.Error("flush w failed: %v", err)
		return err
	}
	return nil
}
