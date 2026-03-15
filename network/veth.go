package network

import (
	"fmt"
	"log"
	"net"

	"github.com/vishvananda/netlink"
)

// SetupVeth creates a veth pair, attaches the host end to the bridge,
// and moves the container end into the container's network namespace.
// Must be called from the host AFTER cmd.Start() so containerPid is valid.
func SetupVeth(containerPid int, containerIP, hostVethName, containerVethName string) error {
	log.Printf("[veth] creating veth pair: host=%s container=%s", hostVethName, containerVethName)

	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: hostVethName,
		},
		PeerName: containerVethName,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("failed to create veth pair: %w", err)
	}

	// Attach host end to the bridge.
	bridge, err := netlink.LinkByName(BridgeName)
	if err != nil {
		netlink.LinkDel(veth)
		return fmt.Errorf("bridge %s not found: %w", BridgeName, err)
	}

	hostVeth, err := netlink.LinkByName(hostVethName)
	if err != nil {
		netlink.LinkDel(veth)
		return fmt.Errorf("failed to find host veth %s: %w", hostVethName, err)
	}

	if err := netlink.LinkSetMaster(hostVeth, bridge); err != nil {
		netlink.LinkDel(veth)
		return fmt.Errorf("failed to attach host veth to bridge: %w", err)
	}

	if err := netlink.LinkSetUp(hostVeth); err != nil {
		netlink.LinkDel(veth)
		return fmt.Errorf("failed to bring up host veth: %w", err)
	}

	// Move container end into the container's network namespace.
	containerVeth, err := netlink.LinkByName(containerVethName)
	if err != nil {
		netlink.LinkDel(veth)
		return fmt.Errorf("failed to find container veth %s: %w", containerVethName, err)
	}

	log.Printf("[veth] moving %s to container namespace (PID %d)", containerVethName, containerPid)
	if err := netlink.LinkSetNsPid(containerVeth, containerPid); err != nil {
		netlink.LinkDel(veth)
		return fmt.Errorf("failed to move veth to container namespace: %w", err)
	}

	log.Printf("[veth] veth pair setup complete: host=%s attached to bridge, container=%s in namespace %d",
		hostVethName, containerVethName, containerPid)
	return nil
}

// ConfigureContainerNetwork configures the network interface inside the container namespace.
// Must be called INSIDE the container namespace (from container/init.go).
func ConfigureContainerNetwork(ip, gateway, ifaceName string) error {
	log.Printf("[veth] configuring container network: iface=%s ip=%s gateway=%s", ifaceName, ip, gateway)

	// Bring up loopback.
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("failed to find loopback: %w", err)
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		return fmt.Errorf("failed to bring up loopback: %w", err)
	}

	// Find the veth interface inside the container.
	veth, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to find interface %s: %w", ifaceName, err)
	}

	// Assign IP address with /16 subnet mask.
	_, ipNet, err := net.ParseCIDR(ip + "/16")
	if err != nil {
		return fmt.Errorf("failed to parse container IP %s: %w", ip, err)
	}
	ipNet.IP = net.ParseIP(ip)

	addr := &netlink.Addr{IPNet: ipNet}
	if err := netlink.AddrAdd(veth, addr); err != nil {
		return fmt.Errorf("failed to assign IP %s to %s: %w", ip, ifaceName, err)
	}

	if err := netlink.LinkSetUp(veth); err != nil {
		return fmt.Errorf("failed to bring up %s: %w", ifaceName, err)
	}

	// Add default route via gateway.
	gw := net.ParseIP(gateway)
	if gw == nil {
		return fmt.Errorf("invalid gateway IP: %s", gateway)
	}

	route := &netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: veth.Attrs().Index,
		Gw:        gw,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("failed to add default route via %s: %w", gateway, err)
	}

	log.Printf("[veth] container network configured: %s/%s gateway %s", ip, "16", gateway)
	return nil
}
