package network

import (
	"errors"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/vishvananda/netlink"
)

const (
	BridgeName = "miniBridge0"
	BridgeCIDR = "172.20.0.1/16"
)

func CreateBridge() error {
	link, err := netlink.LinkByName(BridgeName)
	if err == nil {
		if _, ok := link.(*netlink.Bridge); ok {
			return nil
		}
	}

	bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: BridgeName}}
	if err := netlink.LinkAdd(bridge); err != nil {
		return err
	}

	link, err = netlink.LinkByName(BridgeName)
	if err != nil {
		return err
	}

	_, ipNet, err := net.ParseCIDR(BridgeCIDR)
	if err != nil {
		netlink.LinkDel(link)
		return err
	}
	ipNet.IP = net.ParseIP(BridgeCIDR[:strings.LastIndex(BridgeCIDR, "/")])

	if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: ipNet}); err != nil {
		netlink.LinkDel(link)
		return err
	}

	if err := netlink.LinkSetUp(link); err != nil {
		netlink.LinkDel(link)
		return err
	}

	if err := enableIPForwarding(); err != nil {
		netlink.LinkDel(link)
		return err
	}

	return addMasqueradeRule()
}

func DestroyBridge() error {
	link, err := netlink.LinkByName(BridgeName)
	if err != nil {
		return nil
	}

	_ = removeMasqueradeRule()

	if err := netlink.LinkSetDown(link); err != nil {
		return err
	}

	return netlink.LinkDel(link)
}

func enableIPForwarding() error {
	return os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644)
}

func addMasqueradeRule() error {
	cmd := exec.Command("iptables", "-t", "nat", "-C", "POSTROUTING",
		"-s", Subnet, "!", "-o", BridgeName, "-j", "MASQUERADE")

	if cmd.Run() == nil {
		return nil
	}

	out, err := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
		"-s", Subnet, "!", "-o", BridgeName, "-j", "MASQUERADE").CombinedOutput()
	if err != nil {
		return errors.New("iptables MASQUERADE add failed: " + string(out))
	}
	return nil
}

func removeMasqueradeRule() error {
	out, err := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", Subnet, "!", "-o", BridgeName, "-j", "MASQUERADE").CombinedOutput()
	if err != nil {
		log.Printf("iptables MASQUERADE remove failed (non-fatal): %s", string(out))
	}
	return nil
}
