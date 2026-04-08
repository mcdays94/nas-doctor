package collector

import (
	"strings"

	"github.com/mcdays94/nas-doctor/internal"
)

func collectNetwork() (internal.NetworkInfo, error) {
	info := internal.NetworkInfo{}

	// Get interfaces with ip link show
	out, err := execCmd("ip", "-o", "link", "show")
	if err != nil {
		return info, err
	}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		iface := parseIPLink(line)
		if iface.Name == "" || iface.Name == "lo" {
			continue
		}
		// Skip virtual/tunnel/container interfaces
		if strings.HasPrefix(iface.Name, "veth") ||
			strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "br-") ||
			strings.HasPrefix(iface.Name, "virbr") ||
			strings.HasPrefix(iface.Name, "sit") ||
			strings.HasPrefix(iface.Name, "tunl") ||
			strings.HasPrefix(iface.Name, "ip6tnl") ||
			strings.HasPrefix(iface.Name, "ip6_vti") ||
			strings.HasPrefix(iface.Name, "ip_vti") ||
			strings.HasPrefix(iface.Name, "gre") ||
			strings.HasPrefix(iface.Name, "erspan") ||
			strings.HasPrefix(iface.Name, "ip6gre") ||
			strings.HasPrefix(iface.Name, "dummy") ||
			strings.Contains(iface.Name, "@NONE") {
			continue
		}

		// Try to get speed via ethtool
		if etOut, err := execCmd("ethtool", iface.Name); err == nil {
			for _, eLine := range strings.Split(etOut, "\n") {
				eLine = strings.TrimSpace(eLine)
				if strings.HasPrefix(eLine, "Speed:") {
					iface.Speed = strings.TrimPrefix(eLine, "Speed: ")
					iface.Speed = strings.TrimSpace(iface.Speed)
				}
			}
		}

		// Get IPv4 address
		if addrOut, err := execCmd("ip", "-4", "addr", "show", iface.Name); err == nil {
			for _, aLine := range strings.Split(addrOut, "\n") {
				aLine = strings.TrimSpace(aLine)
				if strings.HasPrefix(aLine, "inet ") {
					fields := strings.Fields(aLine)
					if len(fields) >= 2 {
						iface.IPv4 = fields[1] // includes /prefix
					}
				}
			}
		}

		info.Interfaces = append(info.Interfaces, iface)
	}

	return info, nil
}

func parseIPLink(line string) internal.NetInterface {
	iface := internal.NetInterface{}

	// Format: "2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ..."
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return iface
	}

	// Name is field 1 (remove trailing colon)
	iface.Name = strings.TrimSuffix(fields[1], ":")

	// State from flags
	for _, f := range fields {
		if strings.Contains(f, "UP") && strings.Contains(f, "<") {
			iface.State = "UP"
			break
		}
	}
	if iface.State == "" {
		iface.State = "DOWN"
	}

	// MTU
	for i, f := range fields {
		if f == "mtu" && i+1 < len(fields) {
			mtu := 0
			for _, c := range fields[i+1] {
				if c >= '0' && c <= '9' {
					mtu = mtu*10 + int(c-'0')
				}
			}
			iface.MTU = mtu
			break
		}
	}

	return iface
}
