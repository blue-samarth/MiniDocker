package network

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"

	"github.com/vishvananda/netlink"
)

const (
	BridgeName = "miniDocker0"
	BridgeCIDR = "172.20.0.1/16"
)

// CreateBridge creates the miniDocker0 bridge if it doesn't already exist.
// It is idempotent — safe to call on every container start.
func CreateBridge() error {
	// Check if bridge already exists.
	if link, err := netlink.LinkByName(BridgeName); err == nil {
		if _, ok := link.(*netlink.Bridge); ok {
			log.Printf("[bridge] %s already exists, skipping creation", BridgeName)
			return nil
		}
	}

	log.Printf("[bridge] creating bridge %s with CIDR %s", BridgeName, BridgeCIDR)

	bridge := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: BridgeName,
		},
	}

	if err := netlink.LinkAdd(bridge); err != nil {
		return fmt.Errorf("failed to create bridge %s: %w", BridgeName, err)
	}

	link, err := netlink.LinkByName(BridgeName)
	if err != nil {
		return fmt.Errorf("failed to find bridge after creation: %w", err)
	}

	ip, ipNet, err := net.ParseCIDR(BridgeCIDR)
	if err != nil {
		return fmt.Errorf("failed to parse bridge CIDR %s: %w", BridgeCIDR, err)
	}
	ipNet.IP = ip // use the host address, not the network address

	addr := &netlink.Addr{IPNet: ipNet}
	if err := netlink.AddrAdd(link, addr); err != nil {
		netlink.LinkDel(link)
		return fmt.Errorf("failed to assign address to bridge: %w", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		netlink.LinkDel(link)
		return fmt.Errorf("failed to bring up bridge: %w", err)
	}

	if err := enableIPForwarding(); err != nil {
		return err
	}

	if err := addMasqueradeRule(); err != nil {
		return err
	}

	log.Printf("[bridge] bridge %s created successfully", BridgeName)
	return nil
}

// DestroyBridge removes the miniDocker0 bridge.
func DestroyBridge() error {
	link, err := netlink.LinkByName(BridgeName)
	if err != nil {
		log.Printf("[bridge] bridge %s not found, skipping destroy", BridgeName)
		return nil
	}

	log.Printf("[bridge] destroying bridge %s", BridgeName)

	if err := removeMasqueradeRule(); err != nil {
		log.Printf("[bridge] warning: failed to remove masquerade rule: %v", err)
	}

	if err := netlink.LinkSetDown(link); err != nil {
		return fmt.Errorf("failed to bring down bridge: %w", err)
	}

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete bridge: %w", err)
	}

	log.Printf("[bridge] bridge %s destroyed", BridgeName)
	return nil
}

// enableIPForwarding writes 1 to /proc/sys/net/ipv4/ip_forward.
func enableIPForwarding() error {
	const path = "/proc/sys/net/ipv4/ip_forward"
	log.Printf("[bridge] enabling IP forwarding")
	if err := os.WriteFile(path, []byte("1\n"), 0644); err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w", err)
	}
	return nil
}

// addMasqueradeRule adds an iptables MASQUERADE rule for the bridge subnet.
func addMasqueradeRule() error {
	log.Printf("[bridge] adding iptables MASQUERADE rule for %s", Subnet)
	err := exec.Command("iptables",
		"-t", "nat",
		"-C", "POSTROUTING",
		"-s", Subnet,
		"!", "-o", BridgeName,
		"-j", "MASQUERADE",
	).Run()
	if err == nil {
		// Rule already exists.
		return nil
	}

	out, err := exec.Command("iptables",
		"-t", "nat",
		"-A", "POSTROUTING",
		"-s", Subnet,
		"!", "-o", BridgeName,
		"-j", "MASQUERADE",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add masquerade rule: %w — %s", err, string(out))
	}
	return nil
}

// removeMasqueradeRule removes the iptables MASQUERADE rule.
func removeMasqueradeRule() error {
	log.Printf("[bridge] removing iptables MASQUERADE rule")
	out, err := exec.Command("iptables",
		"-t", "nat",
		"-D", "POSTROUTING",
		"-s", Subnet,
		"!", "-o", BridgeName,
		"-j", "MASQUERADE",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove masquerade rule: %w — %s", err, string(out))
	}
	return nil
}
