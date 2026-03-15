package tests

import (
	"os"
	"testing"

	"miniDocker/network"
)

func TestIPAM_NewIPAM(t *testing.T) {
	subnet := "172.20.0.0/16"
	ipam := network.NewIPAM(subnet)

	if ipam == nil {
		t.Fatal("NewIPAM returned nil")
	}
	if ipam.Subnet != subnet {
		t.Errorf("expected subnet %q, got %q", subnet, ipam.Subnet)
	}
	if len(ipam.List()) != 0 {
		t.Errorf("expected empty allocation map, got %d entries", len(ipam.List()))
	}
}

func TestIPAM_AllocateSequential(t *testing.T) {
	ipam := network.NewIPAM("172.20.0.0/16")

	// First allocation
	ip1, err := ipam.Allocate("container-1")
	if err != nil {
		t.Fatalf("first allocation failed: %v", err)
	}
	if ip1 != "172.20.0.2" {
		t.Errorf("expected first IP to be 172.20.0.2, got %s", ip1)
	}

	// Second allocation
	ip2, err := ipam.Allocate("container-2")
	if err != nil {
		t.Fatalf("second allocation failed: %v", err)
	}
	if ip2 != "172.20.0.3" {
		t.Errorf("expected second IP to be 172.20.0.3, got %s", ip2)
	}

	// Verify both in list
	list := ipam.List()
	if len(list) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(list))
	}
	if list[ip1] != "container-1" {
		t.Errorf("expected container-1 for IP %s, got %s", ip1, list[ip1])
	}
	if list[ip2] != "container-2" {
		t.Errorf("expected container-2 for IP %s, got %s", ip2, list[ip2])
	}
}

func TestIPAM_AllocateUniqueness(t *testing.T) {
	ipam := network.NewIPAM("172.20.0.0/16")

	ips := make(map[string]bool)
	for i := 0; i < 10; i++ {
		containerID := "container-" + string(rune('a'+i))
		ip, err := ipam.Allocate(containerID)
		if err != nil {
			t.Fatalf("allocation %d failed: %v", i, err)
		}

		if ips[ip] {
			t.Errorf("IP %s already allocated", ip)
		}
		ips[ip] = true
	}

	if len(ips) != 10 {
		t.Errorf("expected 10 unique IPs, got %d", len(ips))
	}
}

func TestIPAM_Release(t *testing.T) {
	ipam := network.NewIPAM("172.20.0.0/16")

	// Allocate
	ip1, err := ipam.Allocate("container-1")
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}
	ip2, err := ipam.Allocate("container-2")
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	list := ipam.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 allocations before release")
	}

	// Release first IP
	if err := ipam.Release(ip1); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	list = ipam.List()
	if len(list) != 1 {
		t.Errorf("expected 1 allocation after release, got %d", len(list))
	}
	if _, ok := list[ip1]; ok {
		t.Errorf("IP %s should be released", ip1)
	}
	if list[ip2] != "container-2" {
		t.Errorf("IP %s should still be allocated", ip2)
	}

	// Allocate again - should reuse released IP
	ip3, err := ipam.Allocate("container-3")
	if err != nil {
		t.Fatalf("allocation after release failed: %v", err)
	}
	if ip3 != ip1 {
		t.Errorf("expected to reuse released IP %s, got %s", ip1, ip3)
	}
}

func TestIPAM_ReleaseNonexistent(t *testing.T) {
	ipam := network.NewIPAM("172.20.0.0/16")

	// Release non-existent IP - should not error
	err := ipam.Release("172.20.0.2")
	if err != nil {
		t.Errorf("release of non-existent IP should not error, got %v", err)
	}
}

func TestIPAM_SkipsGateway(t *testing.T) {
	ipam := network.NewIPAM("172.20.0.0/16")

	// Allocate multiple IPs
	for i := 0; i < 5; i++ {
		ip, err := ipam.Allocate("container-" + string(rune('a'+i)))
		if err != nil {
			t.Fatalf("allocation failed: %v", err)
		}
		if ip == network.GatewayIP {
			t.Errorf("allocated gateway IP %s", ip)
		}
	}
}

func TestIPAM_Persistence(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	statePath := "/var/run/miniDocker/ipam_test.json"
	defer os.Remove(statePath)

	// Create IPAM and allocate
	ipam1 := network.NewIPAM("172.20.0.0/16")
	ip1, err := ipam1.Allocate("container-1")
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}
	ip2, err := ipam1.Allocate("container-2")
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	// Create new IPAM instance and verify state loaded
	ipam2 := network.NewIPAM("172.20.0.0/16")
	list := ipam2.List()

	if len(list) != 2 {
		t.Errorf("expected 2 allocations after reload, got %d", len(list))
	}
	if list[ip1] != "container-1" {
		t.Errorf("expected container-1 for IP %s", ip1)
	}
	if list[ip2] != "container-2" {
		t.Errorf("expected container-2 for IP %s", ip2)
	}
}

func TestIPAM_List(t *testing.T) {
	ipam := network.NewIPAM("172.20.0.0/16")

	ip1, _ := ipam.Allocate("container-1")
	_, _ = ipam.Allocate("container-2")

	list := ipam.List()

	if len(list) != 2 {
		t.Errorf("expected 2 entries in list, got %d", len(list))
	}

	// Verify it's a copy (modifying it doesn't affect IPAM)
	list[ip1] = "modified"

	list2 := ipam.List()
	if list2[ip1] != "container-1" {
		t.Errorf("modifying list copy should not affect IPAM state")
	}
}

func TestIPAM_InvalidSubnet(t *testing.T) {
	ipam := network.NewIPAM("invalid-subnet")

	_, err := ipam.Allocate("container-1")
	if err == nil {
		t.Error("expected error for invalid subnet")
	}
}

func TestIPAM_SubnetExhaustion(t *testing.T) {
	// Use a small subnet to test exhaustion
	ipam := network.NewIPAM("172.20.0.0/30")

	// /30 has only 4 IPs total: .0 (network), .1 (gateway), .2, .3
	// Only .2 and .3 are usable

	ip1, err := ipam.Allocate("container-1")
	if err != nil {
		t.Fatalf("first allocation failed: %v", err)
	}

	_, err = ipam.Allocate("container-2")
	if err != nil {
		t.Fatalf("second allocation failed: %v", err)
	}

	// Third allocation should fail
	_, err = ipam.Allocate("container-3")
	if err == nil {
		t.Error("expected error when subnet exhausted")
	}

	// After releasing one, should succeed
	if err := ipam.Release(ip1); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	ip3, err := ipam.Allocate("container-3")
	if err != nil {
		t.Fatalf("allocation after release failed: %v", err)
	}

	if ip3 != ip1 {
		t.Errorf("expected to reuse released IP %s, got %s", ip1, ip3)
	}
}

func TestIPAM_ConcurrentAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}

	ipam := network.NewIPAM("172.20.0.0/16")

	resultCh := make(chan string, 10)
	errorCh := make(chan error, 10)

	// Launch 10 concurrent allocations
	for i := 0; i < 10; i++ {
		go func(id int) {
			ip, err := ipam.Allocate("container-" + string(rune('a'+id)))
			if err != nil {
				errorCh <- err
				return
			}
			resultCh <- ip
		}(i)
	}

	// Collect results
	ips := make(map[string]bool)
	for i := 0; i < 10; i++ {
		select {
		case ip := <-resultCh:
			if ips[ip] {
				t.Errorf("duplicate IP allocated: %s", ip)
			}
			ips[ip] = true
		case err := <-errorCh:
			t.Errorf("concurrent allocation error: %v", err)
		}
	}

	if len(ips) != 10 {
		t.Errorf("expected 10 unique IPs, got %d", len(ips))
	}
}
