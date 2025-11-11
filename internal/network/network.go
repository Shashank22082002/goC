//go:build linux
// +build linux

package network

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

const (
	bridgeName = "goC0"
	bridgeIP   = "10.0.0.1/24"
	// We'll make these dynamic in a moment
	containerIP = "10.0.0.2/24"
	gatewayIP   = "10.0.0.1"
)

// SetupHostSide is called by the Parent.
// It creates the bridge, veth pair, and moves the peer into the container ns.
func SetupHostSide(pid int, vethName, peerName string) error {
	fmt.Printf("[Host] Setting up network for PID %d...\n", pid)

	// 1. Get-or-Create the bridge
	bridge, err := getOrCreateBridge(bridgeName, bridgeIP)
	if err != nil {
		return fmt.Errorf("could not setup bridge: %v", err)
	}

	// 2. Create the veth pair
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: vethName,
		},
		PeerName: peerName,
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("failed to add veth: %v", err)
	}

	// 3. Get the host-side veth link
	hostLink, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("failed to find host veth %s: %v", vethName, err)
	}

	// 4. Add the host-side veth to the bridge
	if err := netlink.LinkSetMaster(hostLink, bridge); err != nil {
		return fmt.Errorf("failed to add veth to bridge: %v", err)
	}

	// 5. Bring up the host-side veth
	if err := netlink.LinkSetUp(hostLink); err != nil {
		return fmt.Errorf("failed to bring up host veth: %v", err)
	}

	// 6. Get the container-side veth (peer)
	peerLink, err := netlink.LinkByName(peerName)
	if err != nil {
		return fmt.Errorf("failed to find peer veth %s: %v", peerName, err)
	}

	// 7. **THE MAGIC**: Move the peer into the container's network namespace
	if err := netlink.LinkSetNsPid(peerLink, pid); err != nil {
		return fmt.Errorf("failed to move peer veth to container ns: %v", err)
	}

	fmt.Printf("[Host] Network setup complete.\n")
	return nil
}

// getOrCreateBridge finds a bridge or creates it.
func getOrCreateBridge(name, ip string) (*netlink.Bridge, error) {
	// 1. Try to find the bridge
	link, err := netlink.LinkByName(name)
	if err == nil {
		// Bridge already exists
		bridge, ok := link.(*netlink.Bridge)
		if !ok {
			return nil, fmt.Errorf("%s already exists but is not a bridge", name)
		}
		// Make sure it's up
		if err := netlink.LinkSetUp(bridge); err != nil {
			return nil, fmt.Errorf("failed to bring up existing bridge: %v", err)
		}
		return bridge, nil
	}

	// 2. If it's a "not found" error, we create it
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return nil, fmt.Errorf("failed to find bridge %s: %v", name, err)
	}

	fmt.Printf("[Host] Bridge %s not found, creating...\n", name)
	// 3. Create the bridge
	bridge := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: name,
		},
	}
	if err := netlink.LinkAdd(bridge); err != nil {
		return nil, fmt.Errorf("failed to create bridge: %v", err)
	}

	// 4. Add an IP to the bridge
	addr, err := netlink.ParseAddr(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bridge IP: %v", err)
	}
	if err := netlink.AddrAdd(bridge, addr); err != nil {
		return nil, fmt.Errorf("failed to add IP to bridge: %v", err)
	}

	// 5. Bring up the bridge
	if err := netlink.LinkSetUp(bridge); err != nil {
		return nil, fmt.Errorf("failed to bring up bridge: %v", err)
	}

	return bridge, nil
}

// SetupContainerSide is called by the Child.
// It configures the "lo" and "eth0" interfaces.
func SetupContainerSide(peerName string) error {
	fmt.Printf("[Container] Setting up network...\n")

	// 1. Bring up the loopback ('lo') interface
	// This is critical for many applications
	loLink, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("failed to find 'lo' interface: %v", err)
	}
	if err := netlink.LinkSetUp(loLink); err != nil {
		return fmt.Errorf("failed to bring up 'lo' interface: %v", err)
	}

	// 2. Find the veth peer (which was moved in by the parent)
	vethLink, err := netlink.LinkByName(peerName)
	if err != nil {
		return fmt.Errorf("failed to find veth peer %s: %v", peerName, err)
	}

	// 3. Rename the veth peer to 'eth0'
	if err := netlink.LinkSetName(vethLink, "eth0"); err != nil {
		return fmt.Errorf("failed to rename peer to 'eth0': %v", err)
	}

	// 4. Set the IP address for 'eth0'
	addr, err := netlink.ParseAddr(containerIP)
	if err != nil {
		return fmt.Errorf("failed to parse container IP: %v", err)
	}
	if err := netlink.AddrAdd(vethLink, addr); err != nil {
		return fmt.Errorf("failed to add IP to 'eth0': %v", err)
	}

	// 5. Bring up the 'eth0' interface
	if err := netlink.LinkSetUp(vethLink); err != nil {
		return fmt.Errorf("failed to bring up 'eth0': %v", err)
	}

	// 6. Add the default route (to the bridge)
	gw := net.ParseIP(gatewayIP)
	route := &netlink.Route{
		Gw: gw,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("failed to add default route: %v", err)
	}

	fmt.Printf("[Container] Network setup complete.\n")
	return nil
}