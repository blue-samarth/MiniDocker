package network

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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

// IPAM manages IP address allocation for containers.
type IPAM struct {
	Subnet    string            // CIDR subnet e.g. "172.20.0.0/16"
	Allocated map[string]string // IP -> ContainerID
	mu        sync.Mutex
	statePath string
}

// NewIPAM creates a new IPAM manager using the default state path,
// loading any persisted state.
func NewIPAM(subnet string) *IPAM {
	return newIPAM(subnet, ipamStatePath)
}

// NewIPAMWithStatePath creates a new IPAM manager with a custom state path.
// Useful for testing without requiring root privileges.
func NewIPAMWithStatePath(subnet, statePath string) *IPAM {
	return newIPAM(subnet, statePath)
}

func newIPAM(subnet, statePath string) *IPAM {
	ipam := &IPAM{
		Subnet:    subnet,
		Allocated: make(map[string]string),
		statePath: statePath,
	}
	if err := ipam.load(); err != nil {
		log.Printf("[ipam] warning: could not load state: %v", err)
	}
	return ipam
}

// Allocate assigns the next available IP to containerID.
// Returns the allocated IP (without subnet mask).
func (ipam *IPAM) Allocate(containerID string) (string, error) {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	if err := ipam.load(); err != nil {
		return "", fmt.Errorf("failed to load ipam state: %w", err)
	}

	_, network, err := net.ParseCIDR(ipam.Subnet)
	if err != nil {
		return "", fmt.Errorf("invalid subnet %q: %w", ipam.Subnet, err)
	}

	// Iterate through the subnet, skipping network address, gateway, and broadcast.
	base := binary.BigEndian.Uint32(network.IP.To4())
	ones, bits := network.Mask.Size()
	size := uint32(1) << uint(bits-ones)

	for i := uint32(2); i < size-1; i++ {
		candidate := make(net.IP, 4)
		binary.BigEndian.PutUint32(candidate, base+i)
		ip := candidate.String()

		// Skip the gateway IP
		if ip == GatewayIP {
			continue
		}

		if _, taken := ipam.Allocated[ip]; !taken {
			ipam.Allocated[ip] = containerID
			if err := ipam.save(); err != nil {
				delete(ipam.Allocated, ip)
				return "", fmt.Errorf("failed to save ipam state: %w", err)
			}
			log.Printf("[ipam] allocated %s to container %s", ip, containerID)
			return ip, nil
		}
	}

	return "", fmt.Errorf("no available IPs in subnet %s", ipam.Subnet)
}

// Release frees the IP address previously allocated to a container.
func (ipam *IPAM) Release(ip string) error {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	if err := ipam.load(); err != nil {
		return fmt.Errorf("failed to load ipam state: %w", err)
	}

	if _, ok := ipam.Allocated[ip]; !ok {
		log.Printf("[ipam] warning: IP %s not found in allocated pool, skipping release", ip)
		return nil
	}

	delete(ipam.Allocated, ip)

	if err := ipam.save(); err != nil {
		return fmt.Errorf("failed to save ipam state after release: %w", err)
	}

	log.Printf("[ipam] released IP %s", ip)
	return nil
}

// List returns a copy of the current allocation map.
func (ipam *IPAM) List() map[string]string {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	copy := make(map[string]string, len(ipam.Allocated))
	for k, v := range ipam.Allocated {
		copy[k] = v
	}
	return copy
}

// load reads persisted IPAM state from disk.
// Silently succeeds if the state file does not exist yet.
func (ipam *IPAM) load() error {
	data, err := os.ReadFile(ipam.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read ipam state file: %w", err)
	}

	allocated := make(map[string]string)
	if err := json.Unmarshal(data, &allocated); err != nil {
		return fmt.Errorf("failed to parse ipam state file: %w", err)
	}

	ipam.Allocated = allocated
	return nil
}

// save atomically writes the current allocation state to disk.
func (ipam *IPAM) save() error {
	if err := os.MkdirAll(filepath.Dir(ipam.statePath), 0755); err != nil {
		return fmt.Errorf("failed to create ipam state dir: %w", err)
	}

	data, err := json.MarshalIndent(ipam.Allocated, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ipam state: %w", err)
	}

	// Write to a temp file then rename for atomicity.
	tmp := ipam.statePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("failed to write ipam temp file: %w", err)
	}

	if err := os.Rename(tmp, ipam.statePath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to rename ipam state file: %w", err)
	}

	return nil
}
