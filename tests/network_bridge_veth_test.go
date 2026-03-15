package tests

import (
	"net"
	"os"
	"testing"

	"miniDocker/network"
)

// Note: Bridge and Veth tests require root privileges and actual network operations
// They are skipped in non-root environments

func TestBridge_Constants(t *testing.T) {
	if network.BridgeName != "miniDocker0" {
		t.Errorf("expected bridge name miniDocker0, got %s", network.BridgeName)
	}
	if network.BridgeCIDR != "172.20.0.1/16" {
		t.Errorf("expected bridge CIDR 172.20.0.1/16, got %s", network.BridgeCIDR)
	}
}

func TestBridge_ParseCIDR(t *testing.T) {
	ip, ipNet, err := net.ParseCIDR(network.BridgeCIDR)
	if err != nil {
		t.Fatalf("failed to parse bridge CIDR: %v", err)
	}

	if ip.String() != "172.20.0.1" {
		t.Errorf("expected IP 172.20.0.1, got %s", ip.String())
	}

	ones, bits := ipNet.Mask.Size()
	if ones != 16 || bits != 32 {
		t.Errorf("expected /16 subnet, got /%d", ones)
	}
}

func TestBridge_CreateBridge(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	// Clean up any existing bridge first
	network.DestroyBridge()

	defer func() {
		if err := network.DestroyBridge(); err != nil {
			t.Logf("cleanup warning: %v", err)
		}
	}()

	// Create bridge
	err := network.CreateBridge()
	if err != nil {
		t.Fatalf("failed to create bridge: %v", err)
	}

	// Verify bridge exists using netlink
	// This is a simple smoke test - real verification happens in integration tests
}

func TestBridge_CreateBridge_Idempotent(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	network.DestroyBridge()
	defer network.DestroyBridge()

	// First creation
	err1 := network.CreateBridge()
	if err1 != nil {
		t.Fatalf("first bridge creation failed: %v", err1)
	}

	// Second creation (should succeed without error, as it's idempotent)
	err2 := network.CreateBridge()
	if err2 != nil {
		t.Fatalf("second bridge creation failed (not idempotent): %v", err2)
	}
}

func TestBridge_DestroyBridge_NonExistent(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	// Ensure bridge doesn't exist
	network.DestroyBridge()

	// Destroying non-existent bridge should not error
	err := network.DestroyBridge()
	if err != nil {
		t.Errorf("destroying non-existent bridge should not error, got %v", err)
	}
}

func TestVeth_ConfigureContainerNetwork_InvalidIP(t *testing.T) {
	// This test doesn't require special privileges, just tests parameter validation

	// Invalid IP should return error
	err := network.ConfigureContainerNetwork("invalid-ip", "172.20.0.1", "eth0")
	if err == nil {
		t.Error("expected error for invalid IP")
	}
}

func TestVeth_ConfigureContainerNetwork_InvalidGateway(t *testing.T) {
	// Invalid gateway should return error
	err := network.ConfigureContainerNetwork("172.20.0.2", "invalid-gateway", "eth0")
	if err == nil {
		t.Error("expected error for invalid gateway")
	}
}

func TestVeth_SetupVeth_InvalidPID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	// Valid parameters but invalid PID should fail
	err := network.SetupVeth(999999, "172.20.0.2", "veth-invalid", "eth0")
	if err == nil {
		t.Error("expected error for invalid PID")
	}
}
