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
func printContainersInfo(infos []*container.Info) error {
	// 格式化输出
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPID\tCOMMAND\tSTATE\tSTARTED_AT\tUPDATED_AT")
	for _, info := range infos {
		state := info.Config.State
		pid := 0
		if info.RuntimeState != nil {
			state = info.RuntimeState.Status
			pid = info.RuntimeState.Pid
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
			info.Config.ID[:12],
			info.Config.Name,
			pid,
			strings.Join(info.Config.Command, " "),
			state,
			info.Config.CreatedAt,
			info.Config.UpdateAt,
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
