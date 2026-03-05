package discovery

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Fingerprint defines a service identification rule.
type Fingerprint struct {
	Name     string   `yaml:"name"`
	Ports    []int    `yaml:"ports"`
	Process  string   `yaml:"process"`
	Category string   `yaml:"category"`
}

type fingerprintFile struct {
	Fingerprints []Fingerprint `yaml:"fingerprints"`
}

// LoadFingerprints loads fingerprint rules from a YAML file.
func LoadFingerprints(path string) ([]Fingerprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseFingerprints(data)
}

// ParseFingerprints parses fingerprint rules from YAML bytes.
func ParseFingerprints(data []byte) ([]Fingerprint, error) {
	var f fingerprintFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Fingerprints, nil
}

// PortProcess represents a listening port with its associated process name.
type PortProcess struct {
	Port    int
	Proto   string
	Process string
}

// Match represents a matched service fingerprint.
type Match struct {
	Fingerprint Fingerprint
	MatchedPort int
	Process     string
}

// MatchServices matches listening ports+processes against fingerprint rules.
// It returns all matches found. A fingerprint matches if:
// - At least one of its ports matches a listening port, AND
// - If the fingerprint specifies a process, that process must be running on the matched port
func MatchServices(fps []Fingerprint, listening []PortProcess) []Match {
	var matches []Match

	for _, fp := range fps {
		for _, lp := range listening {
			if !portIn(lp.Port, fp.Ports) {
				continue
			}
			// If fingerprint requires a specific process, check it
			if fp.Process != "" {
				if !strings.Contains(strings.ToLower(lp.Process), strings.ToLower(fp.Process)) {
					continue
				}
			}
			matches = append(matches, Match{
				Fingerprint: fp,
				MatchedPort: lp.Port,
				Process:     lp.Process,
			})
			break // one match per fingerprint is enough
		}
	}

	return matches
}

func portIn(port int, ports []int) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}
