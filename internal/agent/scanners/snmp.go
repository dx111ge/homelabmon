package scanners

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/dx111ge/homelabmon/internal/models"
	"github.com/dx111ge/homelabmon/internal/plugin"
)

// SNMPScanner discovers devices by querying SNMP-enabled devices on the network.
// Uses SNMPv2c with community string. Walks sysDescr, sysName, and ifPhysAddress.
type SNMPScanner struct {
	community string
	targets   []string // IP addresses or CIDR ranges to probe
}

var _ plugin.Scanner = (*SNMPScanner)(nil)

func NewSNMPScanner(community string, targets []string) *SNMPScanner {
	if community == "" {
		community = "public"
	}
	return &SNMPScanner{community: community, targets: targets}
}

func (s *SNMPScanner) Name() string                  { return "snmp" }
func (s *SNMPScanner) Type() models.PluginType       { return models.PluginTypeScanner }
func (s *SNMPScanner) Detect() bool                  { return len(s.targets) > 0 }
func (s *SNMPScanner) Interval() time.Duration       { return 10 * time.Minute }
func (s *SNMPScanner) Start(_ context.Context) error { return nil }
func (s *SNMPScanner) Stop() error                   { return nil }

// Scan probes each target for SNMP sysName and sysDescr.
func (s *SNMPScanner) Scan(ctx context.Context) ([]plugin.DiscoveredDevice, error) {
	var devices []plugin.DiscoveredDevice

	for _, target := range s.targets {
		// Expand CIDR if needed
		ips := expandTarget(target)
		for _, ip := range ips {
			select {
			case <-ctx.Done():
				return devices, ctx.Err()
			default:
			}

			dev, err := s.probeHost(ip)
			if err != nil {
				continue
			}
			if dev != nil {
				devices = append(devices, *dev)
			}
		}
	}

	return devices, nil
}

// probeHost sends SNMP GET requests for sysName and sysDescr.
func (s *SNMPScanner) probeHost(ip string) (*plugin.DiscoveredDevice, error) {
	addr := net.JoinHostPort(ip, "161")
	conn, err := net.DialTimeout("udp", addr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	// Get sysName (1.3.6.1.2.1.1.5.0)
	sysName, _ := snmpGet(conn, s.community, []int{1, 3, 6, 1, 2, 1, 1, 5, 0})

	if sysName == "" {
		return nil, fmt.Errorf("no response")
	}

	// Get sysDescr (1.3.6.1.2.1.1.1.0)
	sysDescr, _ := snmpGet(conn, s.community, []int{1, 3, 6, 1, 2, 1, 1, 1, 0})

	vendor := LookupVendor("")
	devType := classifySNMP(sysDescr, sysName)

	dev := &plugin.DiscoveredDevice{
		IP:         ip,
		Hostname:   sysName,
		Vendor:     vendor,
		DeviceType: devType,
		Source:     "snmp",
	}

	return dev, nil
}

func classifySNMP(descr, name string) string {
	lower := strings.ToLower(descr + " " + name)
	switch {
	case strings.Contains(lower, "router") || strings.Contains(lower, "cisco") || strings.Contains(lower, "mikrotik"):
		return "router"
	case strings.Contains(lower, "switch") || strings.Contains(lower, "catalyst"):
		return "switch"
	case strings.Contains(lower, "access point") || strings.Contains(lower, "ubiquiti"):
		return "ap"
	case strings.Contains(lower, "printer") || strings.Contains(lower, "laserjet") || strings.Contains(lower, "xerox"):
		return "printer"
	case strings.Contains(lower, "nas") || strings.Contains(lower, "synology") || strings.Contains(lower, "qnap"):
		return "nas"
	case strings.Contains(lower, "linux"):
		return "server"
	case strings.Contains(lower, "windows"):
		return "desktop"
	default:
		return "other"
	}
}

func expandTarget(target string) []string {
	// If it's a CIDR, expand to individual IPs
	if strings.Contains(target, "/") {
		_, ipNet, err := net.ParseCIDR(target)
		if err != nil {
			return []string{target}
		}
		var ips []string
		for ip := ipNet.IP.Mask(ipNet.Mask); ipNet.Contains(ip); incIP(ip) {
			// Skip network and broadcast
			if ip[len(ip)-1] == 0 || ip[len(ip)-1] == 255 {
				continue
			}
			ips = append(ips, ip.String())
		}
		// Limit to /24 max to avoid scanning huge ranges
		if len(ips) > 254 {
			ips = ips[:254]
		}
		return ips
	}
	return []string{target}
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// Minimal SNMPv2c GET implementation (no external dependency).
// Builds a raw SNMP GET-request packet and parses the response.

func snmpGet(conn net.Conn, community string, oid []int) (string, error) {
	pkt := buildSNMPGet(community, oid, 1)
	if _, err := conn.Write(pkt); err != nil {
		return "", err
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	return parseSNMPResponse(buf[:n])
}

// buildSNMPGet constructs a minimal SNMPv2c GET-request packet.
func buildSNMPGet(community string, oid []int, requestID int) []byte {
	// Encode OID
	oidBytes := encodeOID(oid)

	// VarBind: SEQUENCE { OID, NULL }
	varbind := asn1Sequence(append(oidBytes, 0x05, 0x00)) // OID + NULL

	// VarBindList: SEQUENCE { varbind }
	varbindList := asn1Sequence(varbind)

	// RequestID
	reqID := asn1Integer(requestID)
	// Error status = 0
	errStatus := asn1Integer(0)
	// Error index = 0
	errIndex := asn1Integer(0)

	// GetRequest-PDU (0xA0)
	pduContent := append(reqID, errStatus...)
	pduContent = append(pduContent, errIndex...)
	pduContent = append(pduContent, varbindList...)
	pdu := asn1Wrap(0xA0, pduContent)

	// Version: SNMPv2c = 1
	version := asn1Integer(1)

	// Community string
	comm := asn1OctetString([]byte(community))

	// Message: SEQUENCE { version, community, pdu }
	msg := append(version, comm...)
	msg = append(msg, pdu...)
	return asn1Sequence(msg)
}

// parseSNMPResponse extracts the value from the first varbind in an SNMP response.
func parseSNMPResponse(data []byte) (string, error) {
	if len(data) < 10 {
		return "", fmt.Errorf("response too short")
	}

	// Walk through ASN.1 TLV to find the value
	// Structure: SEQUENCE { INTEGER(version), OCTET STRING(community), GetResponse-PDU { ... VarBindList { VarBind { OID, VALUE } } } }
	// We just scan for the last OCTET STRING (0x04) which is typically the value
	for i := len(data) - 1; i >= 2; i-- {
		if data[i-2] == 0x04 && data[i-1] > 0 && data[i-1] < 128 && int(data[i-1])+i <= len(data) {
			length := int(data[i-1])
			if i+length <= len(data) {
				s := strings.TrimSpace(string(data[i : i+length]))
				if len(s) > 0 && isPrintable(s) {
					return s, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no string value found")
}

func isPrintable(s string) bool {
	for _, c := range s {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}

func asn1Sequence(content []byte) []byte {
	return asn1Wrap(0x30, content)
}

func asn1Wrap(tag byte, content []byte) []byte {
	return append([]byte{tag}, append(asn1Length(len(content)), content...)...)
}

func asn1Length(l int) []byte {
	if l < 128 {
		return []byte{byte(l)}
	}
	if l < 256 {
		return []byte{0x81, byte(l)}
	}
	return []byte{0x82, byte(l >> 8), byte(l & 0xFF)}
}

func asn1Integer(v int) []byte {
	if v >= 0 && v < 128 {
		return []byte{0x02, 0x01, byte(v)}
	}
	b := make([]byte, 4)
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
	// Strip leading zeros
	start := 0
	for start < 3 && b[start] == 0 {
		start++
	}
	val := b[start:]
	return append([]byte{0x02, byte(len(val))}, val...)
}

func asn1OctetString(data []byte) []byte {
	return asn1Wrap(0x04, data)
}

func encodeOID(oid []int) []byte {
	if len(oid) < 2 {
		return []byte{0x06, 0x00}
	}
	// First two components encoded as 40*first + second
	content := []byte{byte(40*oid[0] + oid[1])}
	for _, v := range oid[2:] {
		content = append(content, encodeOIDComponent(v)...)
	}
	return append([]byte{0x06, byte(len(content))}, content...)
}

func encodeOIDComponent(v int) []byte {
	if v < 128 {
		return []byte{byte(v)}
	}
	var parts []byte
	for v > 0 {
		parts = append([]byte{byte(v & 0x7F)}, parts...)
		v >>= 7
	}
	for i := 0; i < len(parts)-1; i++ {
		parts[i] |= 0x80
	}
	return parts
}
