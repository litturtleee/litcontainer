package daemon

import "litcontainer/internal/network"

// NetworkCreate 创建网络
func (d *Daemon) NetworkCreate(name, driver, subnet string) error {
	return d.netCtrl.Create(name, driver, subnet)
}

// NetworkList 列出所有网络
func (d *Daemon) NetworkList() ([]*network.Network, error) {
	return d.netCtrl.List()
}

// NetworkInspect 获取网络信息
func (d *Daemon) NetworkInspect(name string) (*network.Network, error) {
	return d.netCtrl.Get(name)
}

// NetworkRemove 删除网络
func (d *Daemon) NetworkRemove(name string) error {
	return d.netCtrl.Delete(name)
}
