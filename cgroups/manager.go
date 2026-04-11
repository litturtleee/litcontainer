package cgroups

import (
	"fmt"
	"litcontainer/pkg/logger"
	"os"
	"path/filepath"
	"strconv"
)

const (
	MemoryMax   = "memory.max"
	CpuMax      = "cpu.max"
	CgroupPorcs = "cgroup.procs"
	CgroupRoot  = "/sys/fs/cgroup"

	CpuPeriod = 100000
)

type CGroupManager struct {
	path string
}

// NewCGroupManager 创建 CGroupManager 实例, 包括创建文件夹、初始化文件等
func NewCGroupManager(path string) *CGroupManager {
	cgroupPath := filepath.Join(CgroupRoot, path)
	if _, err := os.Stat(cgroupPath); os.IsNotExist(err) {
		if err := os.Mkdir(cgroupPath, 0755); err != nil {
			logger.Error("Error creating cgroup: %v", err)
			os.Exit(1)
		}
	}

	return &CGroupManager{
		path: cgroupPath,
	}
}

// Apply 将给定的进程ID（pid）加入到 cgroup 中。
func (c *CGroupManager) Apply(pid int) error {
	pidStr := strconv.Itoa(pid)

	cgroupProcsPath := filepath.Join(c.path, CgroupPorcs)
	if err := os.WriteFile(cgroupProcsPath, []byte(pidStr), 0644); err != nil {
		logger.Error("Error writing pid to cgroup: %v", err)
		return fmt.Errorf("failed to write pid to cgroup: %w", err)
	}
	return nil
}

// SetMemoryLimit 为CGroup设置内存限制
// example: 50m
func (c *CGroupManager) SetMemoryLimit(memoryLimit string) error {
	memoryMaxPath := filepath.Join(c.path, MemoryMax)
	if err := os.WriteFile(memoryMaxPath, []byte(memoryLimit), 0644); err != nil {
		logger.Error("Error writing memory limit to cgroup: %v", err)
		return fmt.Errorf("failed to write memory limit to cgroup: %w", err)
	}

	return nil
}

// SetCPULimit 设置CPU限制
// example --cpus=0.5
func (c *CGroupManager) SetCPULimit(cpusStr string) error {
	quota, period, err := ParseCPUs(cpusStr)
	if err != nil {
		logger.Error("Error parsing cpus: %v", err)
		return err
	}

	cpuMaxPath := filepath.Join(c.path, CpuMax)
	cpuLimit := fmt.Sprintf("%d %d", quota, period)
	if err := os.WriteFile(cpuMaxPath, []byte(cpuLimit), 0644); err != nil {
		logger.Error("Error writing cpu max to cgroup: %v", err)
		return fmt.Errorf("failed to write cpu max to cgroup: %w", err)
	}
	return nil
}

// Cleanup 删除由c.path指定的目录及其所有子目录和文件。
func (c *CGroupManager) Cleanup() error {
	logger.Debug("start cleanup cgroup")
	if err := os.Remove(c.path); err != nil {
		logger.Error("Error removing cgroup: %v", err)
		return err
	}
	return nil
}

// ParseCPUs 将 --cpus 字符串解析为 quota 和 period
// ParseCPUs 解析字符串形式的CPU值，并返回CPU配额（quota）和周期（period）
func ParseCPUs(cpusStr string) (int, int, error) {
	cpusFloat, err := strconv.ParseFloat(cpusStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed parse cpusLimit, %w", ErrCpuLimitInvalid)
	}

	if cpusFloat <= 0 {
		return 0, 0, fmt.Errorf("cpus must be greater than 0, %w", ErrCpuLimitInvalid)
	}

	quota := int(cpusFloat * float64(CpuPeriod))

	if quota <= 0 {
		return 0, 0, fmt.Errorf("calculated quota is invalid: %d, %w", quota, ErrCpuLimitInvalid)
	}

	return quota, CpuPeriod, nil
}
