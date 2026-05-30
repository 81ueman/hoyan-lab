package gitmeta

import (
	"os/exec"
	"strings"
)

type Provider struct{}

func NewProvider() Provider {
	return Provider{}
}

func (Provider) Commit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
