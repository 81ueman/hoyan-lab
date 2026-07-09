package configparse_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestParseNftablesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nftables.conf")
	if err := os.WriteFile(path, []byte(`table inet BLOCK_HTTP_TO_HZ {
  chain forward {
    type filter hook forward priority 0; policy accept;
    oifname "eth1" ip protocol tcp ip daddr 10.4.0.0/16 tcp dport 80 drop
    iifname "eth2" ip protocol icmp ip daddr 10.5.0.0/16 drop
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	acls, bindings, err := configparse.ParseNftablesACLConfig(path)
	if err != nil {
		t.Fatalf("configparse.ParseNftablesACLConfig() error = %v", err)
	}
	if len(acls) != 1 || len(acls[0].Rules) != 2 {
		t.Fatalf("ACLs = %#v, want one ACL with two rules", acls)
	}
	acl := acls[0]
	if acl.Name != "BLOCK-HTTP-TO-HZ" || acl.DefaultAction != model.ACLDefaultPermit {
		t.Fatalf("acl metadata = %#v", acl)
	}
	if acl.Rules[0].Match.Protocol != "tcp" || acl.Rules[0].Action != model.ACLDeny || !acl.Rules[0].Match.DstPort.Contains(80) {
		t.Fatalf("first rule = %#v", acl.Rules[0])
	}
	if len(bindings) != 2 || bindings[0].Direction != "egress" || bindings[0].Interface != "eth1" || bindings[1].Direction != "ingress" || bindings[1].Interface != "eth2" {
		t.Fatalf("bindings = %#v", bindings)
	}
	if acl.Rules[0].Source.Vendor != "nftables" || acl.Rules[0].Source.File != path || acl.Rules[0].Source.Raw == "" {
		t.Fatalf("source = %#v", acl.Rules[0].Source)
	}
}

func TestParseNftablesRejectsUnsupportedStatement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nftables.conf")
	if err := os.WriteFile(path, []byte(`table inet T {
  chain forward {
    type filter hook forward priority 0; policy accept;
    oifname "eth1" ip saddr 10.0.0.0/8 drop
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, _, err := configparse.ParseNftablesACLConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported nftables ip match") {
		t.Fatalf("configparse.ParseNftablesACLConfig() error = %v", err)
	}
}
