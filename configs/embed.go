package configs

import "embed"

//go:embed fingerprints.yaml
var FingerprintsYAML []byte

//go:embed *.yaml
var ConfigFS embed.FS
