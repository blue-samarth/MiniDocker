package network

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
)

const (
	ipamStatePath = "/var/run/miniDocker/ipam.json"
	Subnet        = "172.20.0.0/16"
	GatewayIP     = "172.20.0.1"
)

type IPAM struct {
	Subnet    string
	Allocated map[string]string // ip → containerID
	mu        sync.Mutex
	statePath string
}

func NewIPAM(subnet string) *IPAM {
	return newIPAM(subnet, ipamStatePath)
}

func NewIPAMWithStatePath(subnet, statePath string) *IPAM {
	return newIPAM(subnet, statePath)
}

func newIPAM(subnet, statePath string) *IPAM {
	ipam := &IPAM{
		Subnet:    subnet,
		Allocated: make(map[string]string),
		statePath: statePath,
	}
	_ = ipam.load()
	return ipam
}

func (ipam *IPAM) Allocate(containerID string) (string, error) {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	_ = ipam.load() // refresh

	_, netw, err := net.ParseCIDR(ipam.Subnet)
	if err != nil {
		return "", err
	}

	base := binary.BigEndian.Uint32(netw.IP.To4())
	ones, bits := netw.Mask.Size()
	size := uint32(1) << (bits - ones)

	for i := uint32(2); i < size-1; i++ {
		ipBytes := make(net.IP, 4)
		binary.BigEndian.PutUint32(ipBytes, base+i)
		ip := ipBytes.String()

		if ip == GatewayIP {
			continue
		}

		if _, used := ipam.Allocated[ip]; !used {
			ipam.Allocated[ip] = containerID
			if err := ipam.save(); err != nil {
				delete(ipam.Allocated, ip)
				return "", err
			}
			log.Printf("[ipam] allocated %s → %s", ip, containerID)
			return ip, nil
		}
	}

	return "", errors.New("no free IP in subnet")
}

func (ipam *IPAM) Release(ip string) error {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	_ = ipam.load()

	if _, ok := ipam.Allocated[ip]; !ok {
		return nil
	}

	delete(ipam.Allocated, ip)

	return ipam.save()
}

func (ipam *IPAM) List() map[string]string {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	cp := make(map[string]string, len(ipam.Allocated))
	for ip, id := range ipam.Allocated {
		cp[ip] = id
	}
	return cp
}

func (ipam *IPAM) load() error {
	data, err := os.ReadFile(ipam.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &ipam.Allocated)
}

func (ipam *IPAM) save() error {
	if err := os.MkdirAll(filepath.Dir(ipam.statePath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(ipam.Allocated, "", "  ")
	if err != nil {
		return err
	}

	tmp := ipam.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, ipam.statePath)
}
