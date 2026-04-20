package network

import (
	"fmt"
	"github.com/vishvananda/netlink"
	"litcontainer/enum"
	"litcontainer/pkg/db"
	"litcontainer/pkg/logger"
	"net"
	"os/exec"
	"strings"
	"sync"
)

var mu sync.Mutex

func AllocateIP(subnet *net.IPNet) (net.IP, error) {
	mu.Lock()
	defer mu.Unlock()

	subnetBitmap, err := loadBitmap(subnet.String())
	if err != nil {
		logger.Error("load subnet bitmap failed: %v", err)
		return nil, err
	}

	ip, err := subnetBitmap.AllocateNext()
	if err != nil {
		logger.Error("allocate ip failed: %v", err)
		return nil, err
	}
	logger.Debug("allocate ip: %s", ip)

	err = saveBitmap(subnetBitmap)
	if err != nil {
		logger.Error("save subnet bitmap failed: %v", err)
		return nil, err
	}

	return net.ParseIP(ip).To4(), nil
}

func ReleaseIP(subnet *net.IPNet, ip net.IP) error {
	mu.Lock()
	defer mu.Unlock()

	bitmap, err := loadBitmap(subnet.String())
	if err != nil {
		logger.Error("load subnet bitmap failed: %v", err)
		return err
	}

	if err = bitmap.Clear(ip.String()); err != nil {
		logger.Error("clear ip %s from bitmap failed: %v", ip, err)
		return err
	}

	logger.Info("release ip: %s", ip)
	return saveBitmap(bitmap)
}

func IsAllocatedIP(subnet *net.IPNet, ip net.IP) (bool, error) {
	mu.Lock()
	defer mu.Unlock()

	bitmap, err := loadBitmap(subnet.String())
	if err != nil {
		logger.Error("load subnet bitmap failed: %v", err)
		return false, err
	}

	return bitmap.Has(ip.String())
}

// Stats 统计网络信息
func Stats(subnet *net.IPNet) (used, available int, err error) {
	mu.Lock()
	defer mu.Unlock()

	bitmap, err := loadBitmap(subnet.String())
	if err != nil {
		logger.Error("load subnet bitmap failed: %v", err)
		return 0, 0, err
	}

	return bitmap.Count(), bitmap.Available(), nil
}

func SetInterfaceUp(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		logger.Error("get interface %s failed: %v", name, err)
		return err
	}
	logger.Debug("Link: %v", link)

	err = netlink.LinkSetUp(link)
	if err != nil {
		logger.Error("set interface %s up failed: %v", name, err)
		return err
	}
	return nil
}

func SetInterfaceIP(name, cidr string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		logger.Error("get interface %s failed: %v", name, err)
		return err
	}
	logger.Debug("Link: %v", link)

	ip, err := netlink.ParseAddr(cidr)
	if err != nil {
		logger.Error("parse cidr %s failed: %v", cidr, err)
		return err
	}

	err = netlink.AddrAdd(link, ip)
	if err != nil {
		logger.Error("add ip %s to interface %s failed: %v", cidr, name, err)
		return err
	}
	return nil
}

// SetupSNATIptables 设置 SNAT iptables规则
func SetupSNATIptables(name, srcIpCidr string) error {
	// 所有从子网网段为源的包且不是从name设备名出的包都修改源目标IP
	iptableCmd := fmt.Sprintf("-t nat -A POSTROUTING -s %s ! -o %s -j MASQUERADE", srcIpCidr, name)
	logger.Debug("iptable cmd: %s", iptableCmd)
	output, err := exec.Command("iptables", strings.Split(iptableCmd, " ")...).Output()
	if err != nil {
		logger.Error("exec iptables failed: %v, output: %s", err, output)
		return err
	}
	logger.Info("setup iptables rule: %s", string(output))
	return nil
}

func DeleteSNATIptables(name, srcIpCidr string) error {
	iptableCmd := fmt.Sprintf("-t nat -D POSTROUTING -s %s ! -o %s -j MASQUERADE", srcIpCidr, name)
	logger.Debug("iptable cmd: %s", iptableCmd)
	output, err := exec.Command("iptables", strings.Split(iptableCmd, " ")...).Output()
	if err != nil {
		logger.Error("exec iptables failed: %v, output: %s", err, output)
		return err
	}
	logger.Info("delete iptables rule: %s", string(output))
	return nil
}

// --- 内部方法 ---

func loadBitmap(cidr string) (*SubnetBitmap, error) {
	// new，若数据库有把bitmap的值赋值即可
	subnetBitmap, err := NewSubnetBitmap(cidr)
	if err != nil {
		logger.Error("new subnet bitmap failed: %v", err)
		return nil, err
	}

	var subnetStr []byte
	err = db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		var getErr error
		subnetStr, getErr = dbClient.Get(enum.AllocatedIPKeyTable, cidr)
		return getErr
	})
	if err != nil {
		logger.Error("get subnet bitmap from db failed: %v", err)
		return nil, err
	}
	if subnetStr != nil {
		if err := subnetBitmap.UnmarshalJSON(subnetStr); err != nil {
			logger.Error("unmarshal subnet bitmap failed: %v", err)
			return nil, err
		}
	}
	return subnetBitmap, nil
}

func saveBitmap(subnetBitmap *SubnetBitmap) error {
	key := subnetBitmap.String()
	val, err := subnetBitmap.MarshalJSON()
	if err != nil {
		logger.Error("marshal subnet bitmap failed: %v", err)
		return err
	}

	if err = db.WithBoltDB(enum.DefaultNetworkDBPath, func(dbClient *db.BoltDB) error {
		return dbClient.Put(enum.AllocatedIPKeyTable, key, val)
	}); err != nil {
		logger.Error("save subnet bitmap to db failed: %v", err)
		return err
	}
	return nil
}
