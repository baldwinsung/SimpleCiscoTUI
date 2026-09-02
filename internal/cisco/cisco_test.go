package cisco

import (
	"reflect"
	"testing"
)

const brief = `Interface              IP-Address      OK? Method Status                Protocol
Vlan1                  172.16.0.1      YES NVRAM  up                    up
Vlan10                 10.10.10.1      YES manual up                    up
Vlan20                 10.10.20.1      YES manual administratively down down
GigabitEthernet0/1     unassigned      YES unset  up                    up
GigabitEthernet0/2     unassigned      YES unset  down                  down
`

func TestParseInterfaceBriefCountsAndFields(t *testing.T) {
	rows := ParseInterfaceBrief(brief)
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	vlan1 := rows[0]
	if vlan1.Name != "Vlan1" || vlan1.IP != "172.16.0.1" || vlan1.Status != "up" || vlan1.Protocol != "up" {
		t.Fatalf("unexpected row: %+v", vlan1)
	}
	if !vlan1.IsUp() {
		t.Fatalf("expected Vlan1 to be up")
	}
}

func TestParseInterfaceBriefHandlesAdminDown(t *testing.T) {
	rows := ParseInterfaceBrief(brief)
	var vlan20 *Interface
	for i := range rows {
		if rows[i].Name == "Vlan20" {
			vlan20 = &rows[i]
		}
	}
	if vlan20 == nil {
		t.Fatal("Vlan20 not found")
	}
	// "administratively down" collapses to status="down", protocol="down"
	if vlan20.Status != "down" || vlan20.Protocol != "down" {
		t.Fatalf("unexpected vlan20: %+v", vlan20)
	}
	if vlan20.IsUp() {
		t.Fatal("expected Vlan20 to be down")
	}
}

func TestParseInterfaceBriefIgnoresBlankAndHeader(t *testing.T) {
	if got := ParseInterfaceBrief(""); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
	headerOnly := "Interface              IP-Address      OK? Method Status   Protocol\n"
	if got := ParseInterfaceBrief(headerOnly); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

const interfaceCfg = `Building configuration...

Current configuration : 142 bytes
!
interface Vlan10
 ip address 10.10.10.1 255.255.255.0
 ip access-group ZONE-APP-IN in
 ip access-group ZONE-APP-OUT out
end
`

func TestParseInterfaceAclsBothDirections(t *testing.T) {
	acls := ParseInterfaceAcls(interfaceCfg)
	if acls.Inbound != "ZONE-APP-IN" || acls.Outbound != "ZONE-APP-OUT" {
		t.Fatalf("unexpected acls: %+v", acls)
	}
	if acls.Get("in") != "ZONE-APP-IN" || acls.Get("out") != "ZONE-APP-OUT" {
		t.Fatalf("Get() mismatch: %+v", acls)
	}
	want := []Binding{{"in", "ZONE-APP-IN"}, {"out", "ZONE-APP-OUT"}}
	if got := acls.Bindings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Bindings() = %v, want %v", got, want)
	}
}

func TestParseInterfaceAclsNone(t *testing.T) {
	acls := ParseInterfaceAcls("interface Vlan99\n ip address 10.10.99.1 255.255.255.0\nend")
	if acls.Inbound != "" || acls.Outbound != "" {
		t.Fatalf("expected no acls, got %+v", acls)
	}
	if len(acls.Bindings()) != 0 {
		t.Fatalf("expected no bindings, got %v", acls.Bindings())
	}
}

func TestBuildApplyCommands(t *testing.T) {
	got, err := BuildApplyCommands("Vlan10", "ZONE-APP-IN", "in")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"interface Vlan10", "ip access-group ZONE-APP-IN in"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBuildRemoveCommands(t *testing.T) {
	got, err := BuildRemoveCommands("Vlan10", "ZONE-APP-IN", "out")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"interface Vlan10", "no ip access-group ZONE-APP-IN out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormalizeDirectionAcceptsCaseAndWhitespace(t *testing.T) {
	if got, err := NormalizeDirection(" IN "); err != nil || got != "in" {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := NormalizeDirection("Out"); err != nil || got != "out" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestNormalizeDirectionRejectsGarbage(t *testing.T) {
	if _, err := NormalizeDirection("sideways"); err == nil {
		t.Fatal("expected error for invalid direction")
	}
}
