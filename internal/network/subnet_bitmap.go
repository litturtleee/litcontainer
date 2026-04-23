package network

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"net"
)

type SubnetBitmap struct {
	network *net.IPNet
	base    uint32
	size    int // 子网总数
	bitmap  []uint64
}

func NewSubnetBitmap(cidr string) (*SubnetBitmap, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %s, %w", cidr, err)
	}

	ones, bit := ipNet.Mask.Size()
	size := 1 << (bit - ones)
	bm := &SubnetBitmap{
		network: ipNet,
		base:    ipToUint32(ipNet.IP),
		size:    size,
		bitmap:  make([]uint64, (size+63)/64), // (a+b+1)/b
	}
	// 预标记网络地址和广播地址
	bm.bitmap[0] |= 1 << 0
	bm.bitmap[(size-1)/64] |= 1 << ((size - 1) % 64)
	return bm, nil
}

func (s *SubnetBitmap) GetBitmap() []uint64 {
	return s.bitmap
}

func (s *SubnetBitmap) Set(ip string) error {
	offset, err := s.offset(ip)
	if err != nil {
		return err
	}

	s.bitmap[offset/64] |= 1 << (offset % 64)
	return nil
}

func (s *SubnetBitmap) Clear(ip string) error {
	offset, err := s.offset(ip)
	if err != nil {
		return err
	}
	s.bitmap[offset/64] &^= 1 << (offset % 64)
	return nil
}

func (s *SubnetBitmap) Has(ip string) (bool, error) {
	offset, err := s.offset(ip)
	if err != nil {
		return false, err
	}
	return s.bitmap[offset/64]&(1<<(offset%64)) != 0, nil
}

func (s *SubnetBitmap) AllocateNext() (string, error) {
	for i, b := range s.bitmap {
		if b == ^uint64(0) { // 这 64 个 IP 全满，跳过
			continue
		}
		for j := 0; j < 64; j++ {
			offset := i*64 + j
			if offset >= s.size {
				break
			}
			if b&(1<<j) == 0 {
				s.bitmap[i] |= 1 << j
				return uint32ToIPStr(s.base + uint32(offset)), nil
			}
		}
	}
	return "", fmt.Errorf("subnet %s is exhausted", s.network.String())
}

func (s *SubnetBitmap) Count() int {
	count := 0
	for _, b := range s.bitmap {
		count += bits.OnesCount64(b)
	}
	return count
}

func (s *SubnetBitmap) Available() int {
	return s.size - s.Count()
}

func (s *SubnetBitmap) String() string {
	return s.network.String()
}

type subnetState struct {
	Words []uint64 `json:"words"`
}

func (s *SubnetBitmap) MarshalJSON() ([]byte, error) {
	return json.Marshal(subnetState{Words: s.bitmap})
}

// UnmarshalJSON
// 只需要将bitmap的内容序列化和反序列化就行(因为当需要load的时候key就是cidr的string，可以再new一个出来，然后赋值bitmap就行)
func (s *SubnetBitmap) UnmarshalJSON(data []byte) error {
	var state subnetState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	s.bitmap = state.Words
	return nil
}

// --- 内部工具函数 ---

func (s *SubnetBitmap) offset(ipStr string) (int, error) {
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid IPv4 address: %s", ipStr)
	}
	if !s.network.Contains(ip) {
		return 0, fmt.Errorf("IP %s is not in subnet %s", ipStr, s.network)
	}
	return int(ipToUint32(ip) - s.base), nil
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIPStr(n uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", n>>24, (n>>16)&0xFF, (n>>8)&0xFF, n&0xFF)
}
