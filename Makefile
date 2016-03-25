COMPS=arp\
		bgp\
		ospf\
		rib\
		dhcp_relay\
		bfd \
		vrrp\
		tunnel/vxlan

IPCS=arp\
	  bgp\
	  ospf\
	  rib\
	  dhcp_relay\
	  bfd\
	  vrrp\
	tunnel/vxlan

all: ipc exe install

exe: $(COMPS)
	 $(foreach f,$^, make -C $(f) exe;)

ipc: $(IPCS)
	 $(foreach f,$^, make -C $(f) ipc;)

clean: $(COMPS)
	 $(foreach f,$^, make -C $(f) clean;)

install: $(COMPS)
	 $(foreach f,$^, make -C $(f) install;)

