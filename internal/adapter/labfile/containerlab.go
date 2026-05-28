package labfile

import (
	"os"

	"gopkg.in/yaml.v3"
)

type File struct {
	Name   string  `yaml:"name"`
	Prefix *string `yaml:"prefix"`
	Mgmt   struct {
		IPv4Subnet string `yaml:"ipv4-subnet"`
		Network    string `yaml:"network"`
	} `yaml:"mgmt"`
	Topology struct {
		Nodes map[string]Node `yaml:"nodes"`
		Links []Link          `yaml:"links"`
	} `yaml:"topology"`
}

type Node struct {
	Kind          string   `yaml:"kind"`
	Group         string   `yaml:"group"`
	NetworkMode   string   `yaml:"network-mode"`
	MgmtIPv4      string   `yaml:"mgmt-ipv4"`
	Binds         []string `yaml:"binds"`
	StartupConfig string   `yaml:"startup-config"`
}

type Link struct {
	Endpoints []string `yaml:"endpoints"`
}

func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var raw File
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return File{}, err
	}
	return raw, nil
}
