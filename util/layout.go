package util

import (
	"io/ioutil"
	"log"
	"os"
	"sort"

	yaml "github.com/ghodss/yaml"
)

// Route defines a static route to be used if setting up policy routes.
type Route struct {
	// From is the source address (or address range) of the route.
	From *IP `json:"from,omitempty"`
	// To is the destination address (or address range) of the route.
	To *IP `json:"to,omitempty"`
	// Via is the address that packets taking this route should be sent
	// to.
	Via *IP `json:"via,omitempty"`
	// OnLink has the kernel skip the reachability check for the Via
	// address.
	OnLink bool `json:"on-link,omitempty"`
	// Metric is the route metric.  It defaults to 100 of omitted.
	Metric int `json:"metric,omitempty"`
	// Type is the type of the route.  It can be one of 'unicast',
	// 'unreachable','blackhole', or 'prohibit'.  If omitted, defaults
	// to 'unicast'
	Type string `json:"type,omitempty"`
	// Scope is the scope of the route.  It can be one of
	// 'global','link', or 'host'.  If omitted, defaults to 'global'
	Scope string `json:"scope,omitempty"`
	// Table is the table the route should be inserted into, if you want
	// something other than the default table for the route type.
	Table int `json:"table,omitempty"`
}

func (r *Route) validate() error {
	e := &Err{Prefix: "Route"}
	if r.Via != nil && r.Via.IsCIDR() {
		e.Errorf("Via must be a single IP address, not %s", r.Via)
	}
	switch r.Type {
	case "unicast":
		if r.To == nil || r.Via == nil {
			e.Errorf("unicast routes require 'to' and 'via'")
		}
	default:
		if r.To == nil {
			e.Errorf("%s routes require 'to'", r.Type)
		}
	}
	return e.OrNil()
}

// RoutePolicy defines packet handling policies for specific routes
type RoutePolicy struct {
	// From specifies the source address (or address range) to match.
	// If omitted, any source address matches.
	From *IP `json:"from,omitempty"`
	// To specifies a destination address (or address range) to match.
	// If omitted, any destination address matches.
	To *IP `json:"to,omitempty"`
	// Table specified the routing table to use if a packet matches.
	Table int `json:"table,omitempty"`
	// Priority specifies the priority of the route policy. The lower
	// the number, the higher the priority.
	Priority int `json:"priority,omitempty"`
	// FWMark (if set) specifies that a packet must have a matching mark
	// from the firewall to be considered.
	FWMark int `json:"mark,omitempty"`
	// TOS is (if set) the value of the TOS field that the packet must
	// have to be considered for this policy.
	TOS int `json:"type-of-service,omitempty"`
}

func (r *RoutePolicy) validate() error {
	e := &Err{Prefix: "RoutePolicy"}
	if (r.From != nil) == (r.To != nil) {
		e.Errorf("Route policy must include either a From or a To")
	}

	return e.OrNil()
}

type NSInfo struct {
	Search    []string `json:"search,omitempty"`
	Addresses []*IP    `json:"addresses,omitempty"`
}

// Network defines the layer 3 network configuration that a specific
// interface should have.
type Network struct {
	// Dhcp4 specifies whether an IPv4 address should be solicited for
	// this interface via DHCP.
	Dhcp4 bool `json:"dhcp4,omitempty"`
	// Dhcp6 specifies whether an IPv6 address should be solicited for
	// this interface via DHCP6
	Dhcp6 bool `json:"dhcp6,omitempty"`
	// DHcpIdentifier specifies what should be used as a unique
	// identifier for this interface when performing DHCP operations.
	// If unset, a generated Client ID will be used.  THe only other
	// valid value is 'mac' which specifies that the MAC address on the
	// interface should be used.
	DhcpIdentifier string `json:"dhcp-identifier,omitempty"`
	// AcceptRa signals that the interface should get an IPv6 address by
	// autogenerating one in response to an IPv6 router advertisement
	// packet.
	AcceptRa bool `json:"accept-ra,omitempty"`
	// Addresses is a list of IP addresses in CIDR format that should be
	// assigned to this interface.  If this list is set and the DHCP
	// flags are also set, these addresses and the DHCP addresses will
	// be added to the interface.
	Addresses []*IP `json:"addresses,omitempty"`
	// Gateway4 is the IPv4 default gateway address that should be set
	// for this interface.
	Gateway4 *IP `json:"gateway4,omitempty"`
	// Gateway6 is the IPv6 default gateway address that should be set
	// for this interface.
	Gateway6 *IP `json:"gateway6,omitempty"`
	// Nameservers defines what DNS name servers and search domains
	// should be used.
	Nameservers *NSInfo `json:"nameservers,omitempty"`
	// Routes defines any additional routes that should be added for
	// this interface when it s brought up.
	Routes []Route `json:"routes,omitempty"`
	// RoutingPolicy defines additional routing policy entries that
	// should be added when this interface is brought up.
	RoutingPolicy []RoutePolicy `json:"routing-policy,omitempty"`
}

func (n *Network) configure() bool {
	return n != nil
}

// SetupStaticOnly returns true if this Network should be configured
// using static addressing without DHCP.
func (n *Network) SetupStaticOnly() bool {
	return n.configure() && !(n.Dhcp4 || n.Dhcp6) && len(n.Addresses) > 0
}

// Configure returns true if this Network should be configured.
func (n *Network) Configure() bool {
	return n.configure() && (n.Dhcp4 || n.Dhcp6 || len(n.Addresses) != 0)
}

// SetupDHCPOnly returns true if this Network should be configured via DHCP.
func (n *Network) SetupDHCPOnly() bool {
	return n.configure() && (n.Dhcp4 || n.Dhcp6) && len(n.Addresses) == 0
}

func (n *Network) validate() error {
	e := &Err{Prefix: "network"}
	ValidateStrIn(e, "dhcp-identifier", n.DhcpIdentifier, "mac", "")
	if n.Addresses == nil {
		n.Addresses = []*IP{}
	}
	ValidateIPList(e, "addresses", n.Addresses, true)
	if n.Gateway4 != nil && n.Gateway4.IP.To4() == nil {
		e.Errorf("Gateway4 %s is not an IPv4 address", n.Gateway4)
	}
	if n.Gateway6 != nil && n.Gateway6.IP.To4() != nil {
		e.Errorf("Gateway6 %s is not an IPv6 address", n.Gateway6)
	}
	if n.Nameservers.Addresses != nil {
		ValidateIPList(e, "nameservers", n.Nameservers.Addresses, false)
	}
	if n.Routes != nil {
		for _, route := range n.Routes {
			e.Merge(route.validate())
		}
	}
	if n.RoutingPolicy != nil {
		for _, rp := range n.RoutingPolicy {
			e.Merge(rp.validate())
		}
	}
	return e.OrNil()
}

func validatePhysical(l *Layout, i *Interface, e *Err) {
	// a physical interface can belong to 1 bond, 1 bridge, or n other interface types
	// physical interfaces can not own any other interfaces
	if len(i.CurrentHwAddr) != 0 {
		e.Errorf("Physical interfaces must have CurrentHwAddr set")
	}
	if len(i.Interfaces) != 0 {
		e.Errorf("Physical interfaces must have no sub interfaces")
	}
	l.Roots = append(l.Roots, i.Name)
}

func validateExclusive(l *Layout, i *Interface, e *Err) {
	if len(i.Interfaces) == 0 {
		l.Roots = append(l.Roots, i.Name)
		return
	}
	switch i.Type {
	case "bridge", "bond":
		for _, name := range i.Interfaces {
			intf := l.Interfaces[name]
			if others, ok := l.Child2Parent[name]; ok {
				e.Errorf("Sub interface %s already owned by %v", name, others)
				return
			}
			switch i.Type {
			case "bond":
				if intf.Type != "physical" {
					e.Errorf("%s:%s cannot have non-physical sub interface %s:%s", i.Type, i.Name, intf.Type, intf.Name)
					continue
				}
			case "bridge":
				if intf.Type == "bridge" {
					e.Errorf("%s:%s cannot have %s:%s as a sub interface", i.Type, i.Name, intf.Type, intf.Name)
					continue
				}
			}
			l.Child2Parent[name] = []string{i.Name}
			intf.Network = nil
		}
	default:
		log.Panicf("Interface %s:%s cannot validate as an exclusive owner", i.Type, i.Name)
	}
}

func validateShared(l *Layout, i *Interface, e *Err) {
	switch i.Type {
	case "bridge", "bond", "physical":
		log.Panicf("Interface %s:%s cannot validate as a shared owner", i.Type, i.Name)
	default:
		if len(i.Interfaces) != 1 {
			e.Errorf("Interface %s:%s must belong to exactly 1 interface!", i.Type, i.Name)
			return
		}
	}
	master := l.Interfaces[i.Interfaces[0]]
	// This is overly restrictive, but will do for now.
	switch master.Type {
	case "bridge", "bond", "physical":
	default:
		e.Errorf("Cannot build %s:%s on interface %s:%s", i.Type, i.Name, master.Type, master.Name)
		return
	}
	others, ok := l.Child2Parent[master.Name]
	if !ok {
		l.Child2Parent[master.Name] = []string{i.Name}
		return
	}
	for _, name := range others {
		intf := l.Interfaces[name]
		switch intf.Type {
		case "bridge", "bond":
			e.Errorf("Interface %s:%s cannot belong to %s:%s")
			continue
		}
	}
	if e.Empty() {
		others = append(others, i.Name)
		sort.Strings(others)
		l.Child2Parent[master.Name] = others
	}
}

// Interface carries all the information needed to configure any
// network interfaces that may be required by the Layout.
type Interface struct {
	// Type is the type of Interface.  Valid values are
	// 'physical','bond','bridge', and 'vlan'.  Additional interface
	// types may be added as needed.
	Type string `json:"type"`
	// MatchID is the ID that the interface was identified as from the
	// input format.  It is permitted to have multiple Interfaces with
	// the same MatchID if those Interfaces are physical -- that
	// indicates that the input interface matched multiple physical
	// Interfaces.  All other Interfaces mustt have unique MatchID
	// fields.
	MatchID string `json:"match-id"`
	// Name is the final name that the interface should have. We
	// currently do not support renaming physical interfaces.  The
	// Read() function on the input formats is responsible for any
	// translation needed to turn a MatchID into a Name (or series of
	// interfaces with unique Names).  All Interfaces must have unique
	// Names.
	Name string `json:"name"`
	// CurrentHwAddr is the MAC address of a physical interface.  The
	// Read() function of the input format is responsible for setting
	// this to a proper value.
	CurrentHwAddr HardwareAddr `json:"hwaddr,omitempty"`
	// Optional indicates to the output format that this interface is
	// not required to be present or created for it to finish bringing
	// up the network.  Optionality bubbles upwards from child to
	// parent.
	Optional bool `json:"optional,omitempty"`
	// Interfaces holds the names of other Interfaces that the current
	// Interface will build upon.
	Interfaces []string `json:"interfaces,omitempty"`
	// Parameters contains any additional parameters that may be needed
	// to configure this interface.  Different interface Types have
	// different Parameters.
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	// Network contains the layer3 network configuration that should be
	// applied to this Interface once it is brought up, if any.
	Network *Network `json:"network,omitempty"`
}

func NewInterface() Interface {
	return Interface{
		Interfaces: []string{},
		Parameters: map[string]interface{}{},
	}
}

func (i *Interface) validate(l *Layout) error {
	e := &Err{Prefix: i.Type + ":" + i.MatchID}
	if i.Interfaces == nil {
		i.Interfaces = []string{}
	}
	if i.Type == "physical" {
		if len(i.Interfaces) > 0 {
			e.Errorf("%s:%s must not refer to sub interfaces %v", i.Type, i.Name, i.Interfaces)
		}
		l.Roots = append(l.Roots, i.Name)
		return e.OrNil()
	}
	sort.Strings(i.Interfaces)
	for _, name := range i.Interfaces {
		child, ok := l.Interfaces[name]
		if !ok {
			e.Errorf("%s:%s refers to undefined sub interface %s", i.Type, i.Name, name)
			continue
		}
		switch i.Type {
		case "bond":
			if child.Type != "physical" {
				e.Errorf("%s:%s refers to %s:%s, which is not a physical interface.", i.Type, i.Name, child.Type, child.Name)
				continue
			}
		case "bridge":
			if child.Type == "bridge" {
				e.Errorf("%s:%s cannot be built on %s:%s", i.Type, i.Name, child.Type, child.Name)
				continue
			}
		case "vlan":
			if child.Type == "vlan" {
				e.Errorf("%s:%s cannot be built on %s:%s", i.Type, i.Name, child.Type, child.Name)
				continue
			}
		default:
			log.Panicf("Cannot happen handling %s:%s -> %s:%s", child.Type, child.Name, i.Type, i.Name)
		}
		otherNames, ok := l.Child2Parent[name]
		if !ok {
			l.Child2Parent[name] = []string{i.Name}
			continue
		}
		for _, otherName := range otherNames {
			other := l.Interfaces[otherName]
			switch i.Type {
			case "bridge", "bond":
				e.Errorf("%s:%s is already owned by %s:%s, it canot be a member of %s:%s",
					child.Type, child.Name,
					other.Type, other.Name,
					i.Type, i.Name)
				continue
			case "vlan":
				if other.Type != "vlan" {
					e.Errorf("%s:%s is already owned by %s:%s, it canot be a member of %s:%s",
						child.Type, child.Name,
						other.Type, other.Name,
						i.Type, i.Name)
					continue
				}
				l.Child2Parent[child.Name] = append(l.Child2Parent[child.Name], i.Name)
				sort.Strings(l.Child2Parent[child.Name])
			default:
				log.Panicf("Cannot happen handling %s:%s <-> %s:%s", child.Type, child.Name, other.Type, other.Name)
			}
		}
	}
	return e.OrNil()
}

// Layout is the intermediate data format that netwrangler uses as an
// intermediate step between input formats and output formats.
type Layout struct {
	// Interfaces contains all the Interface definitions that are
	// required to create this Layout.  It must be complete -- the
	// Interfaces fields in each individual Interface in this map must
	// resolve to another Interface in this map.  The Read() function
	// of the input formats is responsible for making sure this
	// condition is met.
	Interfaces map[string]Interface
	// Child2Parent records the topological order in which interfaces
	// must be created and/or brought up.
	Child2Parent map[string][]string
	// Roots contains the names of interfaces that must be brought up or
	// created first.
	Roots []string
}

func (l *Layout) Read(src string) (*Layout, error) {
	in := os.Stdin
	if src != "" {
		i, e := os.Open(src)
		if e != nil {
			return nil, e
		}
		defer i.Close()
		in = i
	}
	buf, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, err
	}
	return l, yaml.Unmarshal(buf, l)
}

func (l *Layout) Write(dest string) error {
	out := os.Stdout
	if dest != "" {
		o, e := os.Create(dest)
		if e != nil {
			return e
		}
		defer o.Close()
		out = o
	}
	buf, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	_, err = out.Write(buf)
	return err
}

func (l *Layout) cyclic(intf string, working []string, clean map[string]struct{}, e *Err) {
	if _, ok := clean[intf]; ok {
		// We already know that this interface is not part of a cycle.
		return
	}
	next, found := l.Child2Parent[intf]
	if !found {
		// We hit the end of a branch.  Mark all working nodes as clean.
		for _, n := range working {
			clean[n] = struct{}{}
		}
		return
	}
	for _, t := range working {
		if t == intf {
			e.Errorf("%s: Cycle detected: %v", intf, working)
			return
		}
	}
	working = append(working, intf)
	for _, n := range next {
		l.cyclic(n, working, clean, e)
	}
}

// Validate validates that the Layout describes a sane network
// configuration.  It must be called by any Reader in the
// implemntation of its Read() method.
func (l *Layout) Validate() error {
	e := &Err{Prefix: "layout"}
	l.Roots = []string{}
	l.Child2Parent = map[string][]string{}
	members := []string{}
	for k := range l.Interfaces {
		members = append(members, k)
	}
	sort.Strings(members)
	for _, k := range members {
		v := l.Interfaces[k]
		v.validate(l)
	}
	if !e.Empty() {
		return e
	}
	cleanInterfaces := map[string]struct{}{}
	for k := range l.Interfaces {
		l.cyclic(k, []string{}, cleanInterfaces, e)
	}
	sort.Strings(l.Roots)
	return e.OrNil()
}
