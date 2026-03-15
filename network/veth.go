package network

import (
	"errors"
	"log"
	"net"

	"github.com/vishvananda/netlink"
)

// SetupVeth creates veth pair, attaches host end to bridge, moves container end to container namespace.
func SetupVeth(containerPid int, containerIP, hostVethName, containerVethName string) error {
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: hostVethName},
		PeerName:  containerVethName,
	}

	if err := netlink.LinkAdd(veth); err != nil {
		return err
	}

	bridge, err := netlink.LinkByName(BridgeName)
	if err != nil {
		_ = netlink.LinkDel(veth)
		return err
	}

	hostEnd, err := netlink.LinkByName(hostVethName)
	if err != nil {
		_ = netlink.LinkDel(veth)
		return err
	}

	if err := netlink.LinkSetMaster(hostEnd, bridge); err != nil {
		_ = netlink.LinkDel(veth)
		return err
	}

	if err := netlink.LinkSetUp(hostEnd); err != nil {
		_ = netlink.LinkDel(veth)
		return err
	}

	containerEnd, err := netlink.LinkByName(containerVethName)
	if err != nil {
		_ = netlink.LinkDel(veth)
		return err
	}

	if err := netlink.LinkSetNsPid(containerEnd, containerPid); err != nil {
		_ = netlink.LinkDel(veth)
		return err
	}

	log.Printf("veth ready: host=%s → bridge, container=%s in pid %d", hostVethName, containerVethName, containerPid)
	return nil
}

// ConfigureContainerNetwork sets up loopback, assigns IP, adds default route (called inside container).
func ConfigureContainerNetwork(ip, gateway, ifaceName string) error {
	// Bring up loopback
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return err
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		return err
	}

	veth, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return err
	}

	_, ipNet, err := net.ParseCIDR(ip + "/16")
	if err != nil {
		return err
	}
	ipNet.IP = net.ParseIP(ip)

	if err := netlink.AddrAdd(veth, &netlink.Addr{IPNet: ipNet}); err != nil {
		return err
	}

	if err := netlink.LinkSetUp(veth); err != nil {
		return err
	}

	gw := net.ParseIP(gateway)
	if gw == nil {
		return errors.New("invalid gateway IP")
	}

	route := &netlink.Route{
		Scope:     netlink.SCOPE_UNIVERSE,
		LinkIndex: veth.Attrs().Index,
		Gw:        gw,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return err
	}

	log.Printf("container network: %s/16 via %s on %s", ip, gateway, ifaceName)
	return nil
}
