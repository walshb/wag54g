#!/bin/sh

set -eu

clean () {
	rm -rf rootfs
	rm -f firmware-code.bin
	rm -f fs.img
	rm -f src/linux-atm-2.5.2.tar.gz
	rm -rf src/linux-atm-2.5.2
	rm -f src/sangam_atm-D7.05.01.00-R1.tar.bz2
	rm -rf src/sangam_atm-D7.05.01.00/
	rm -rf tools
	rm -f vmlinuz.bin
	rm -f vmlinuz.srec

	rm -rf src/buildroot/output
}

ar7flashtools () {
	mkdir -p tools

	[ -x tools/srec2bin ] \
		|| gcc -o tools/srec2bin src/openwrt/tools/firmware-utils/src/srec2bin.c
	[ -x tools/addpattern ] \
		|| gcc -o tools/addpattern src/openwrt/tools/firmware-utils/src/addpattern.c
}

buildroot () {
	ln -s -f -T ../../config/buildroot.config src/buildroot/.config

	mkdir -p dl
	ln -s -f -t src/buildroot ../../dl

	export UCLIBC_CONFIG_FILE=../../config/uclibc.config 
	export BUSYBOX_CONFIG_FILE=../../config/busybox.config

	make -C src/buildroot oldconfig
	make -C src/buildroot

	rsync -rl src/buildroot/output/target/ rootfs
}

interfaces () {
	if [ "$WAN6IP" = "${WAN6IP#2002:}" ]; then
		V6REAL="up"
		V66TO4="#up"
	else
		V6REAL="#up"
		V66TO4="up"
	fi

	cat <<EOF > rootfs/etc/network/interfaces
# Configure Loopback
auto lo
iface lo inet loopback
	up	modprobe ipv6

	up	sysctl -q -w net.ipv4.conf.default.rp_filter=1
	up	sysctl -q -w net.ipv4.conf.all.rp_filter=1
	up	sysctl -q -w net.ipv4.conf.lo.rp_filter=1

	up	sysctl -q -w net.ipv4.conf.default.forwarding=1
	up	sysctl -q -w net.ipv4.conf.all.forwarding=1
	up	sysctl -q -w net.ipv4.ip_forward=1

	up	iptables-restore < /etc/network/iptables.active

	up	sysctl -q -w net.netfilter.nf_conntrack_max=4096

	# bogons (rfc6890)
	up	ip route add unreachable 10.0.0.0/8
	up	ip route add unreachable 100.64.0.0/10
	up	ip route add unreachable 169.254.0.0/16
	up	ip route add unreachable 172.16.0.0/12
	up	ip route add unreachable 192.0.0.0/24
	up	ip route add unreachable 192.0.2.0/24
	#$V66TO4	ip route add unreachable 192.88.99.0/24
	up	ip route add unreachable 192.168.0.0/16
	up	ip route add unreachable 198.18.0.0/15
	up	ip route add unreachable 198.51.100.0/24
	up	ip route add unreachable 203.0.113.0/24
	up	ip route add blackhole 240.0.0.0/4
iface lo inet6 static
	address	$WAN6NT::
	netmask	64

	up	sysctl -q -w net.ipv6.conf.default.forwarding=1
	up	sysctl -q -w net.ipv6.conf.all.forwarding=1

	up	sysctl -q -w net.ipv6.conf.default.autoconf=0
	up	sysctl -q -w net.ipv6.conf.all.autoconf=0

	# bogons (rfc6890)
	up	ip route add unreachable 64:ff9b::/96
	up	ip route add unreachable ::ffff:0:0/96
	up	ip route add unreachable 100::/64
	up	ip route add unreachable 2001::/23
	up	ip route add unreachable 2001:2::/48
	up	ip route add unreachable 2001:db8::/32
	up	ip route add unreachable 2001:10::/28
	#$V6REAL	ip route add unreachable 2002::/16
	up	ip route add unreachable fc00::/7

	# blackhole our allocation to prevent loops
	up	ip route add unreachable $WAN6NT::/48

	up	ip6tables-restore < /etc/network/ip6tables.active

auto eth0
iface eth0 inet static
	pre-up	modprobe cpmac
	address	$LAN4IP
	netmask	$LAN4SN
iface eth0 inet6 static
	address	$WAN6NT:$LAN6NT::
	netmask	64
EOF

	if [ "$TYPE" = "PPPoE" ]; then
		cat <<EOF >> rootfs/etc/network/interfaces

iface nas0 inet manual
	pre-up	br2684ctl -c 0 -a $VPI.$VCI -e $ENCAP -b
	up	ip link set dev nas0 up
	down	pkill br2684ctl
EOF
fi

	if [ "$WAN6IP" != "${WAN6IP#2002:}" ]; then
		cat <<EOF >> rootfs/etc/network/interfaces

iface tun6to4 inet6 v4tunnel
	address $WAN6IP
	netmask 16
	gateway ::192.88.99.1
	endpoint any
	local $WAN4IP
EOF
	fi
}

accounts () {
	sed -i "/^root:/ s/^root:[^:]*:/root:$ROOTPASSWD:/" rootfs/etc/shadow

	sed -i '/^default:/ d' rootfs/etc/passwd rootfs/etc/group rootfs/etc/shadow

	I=1000

	for ACCT in $(find . -maxdepth 1 -name 'ssh.*' | sort); do
		N=$(echo "$ACCT" | cut -d. -f3)

		cat <<EOF >> rootfs/etc/passwd
$N:x:$I:$I::/home/$N:/bin/sh
EOF
		cat <<EOF >> rootfs/etc/group
$N:x:$I:
EOF
		cat <<EOF >> rootfs/etc/shadow
$N:*:::::::
EOF

		mkdir -p "rootfs/home/$N/.ssh"
		cat "$ACCT" > "rootfs/home/$N/.ssh/authorized_keys"

		cat <<EOF >> "$DEVTABLE"
/home/$N			d	0700	$I	$I	-	-	-	-	-
/home/$N/.ssh			d	0700	$I	$I	-	-	-	-	-
/home/$N/.ssh/authorized_keys	f	0600	$I	$I	-	-	-	-	-
EOF

		I=$((I+1))
	done
}

customise () {
	rsync -rl overlay/ rootfs

	echo -n $HOSTNAME > rootfs/etc/hostname
	sed -i "s/%HOSTNAME%/$HOSTNAME/g; \
			s/%DOMAIN%/$DOMAIN/; \
			s/%LAN4IP%/$LAN4IP/; \
			s/%WAN4IP%/$WAN4IP/; \
			s/%WAN6NT%/$WAN6NT/;" rootfs/etc/hosts

	interfaces
	accounts

	sed -i "s/%NTP%/$NTP/" rootfs/etc/sv/ntpd/run

	sed -i "s/%USER%/$USER/; s/%PASS%/$PASS/" rootfs/etc/ppp/options
	if [ "$TYPE" = "PPPoE" ]; then
		echo plugin rp-pppoe.so nas0 >> rootfs/etc/ppp/options
	else
		echo plugin pppoatm.so $VPI.$VCI >> rootfs/etc/ppp/options
	fi
	[ "$WAN6IP" = "${WAN6IP#2002:}" ] && echo +ipv6 >> rootfs/etc/ppp/options

	sed -i "s/%WAN4IP%/$WAN4IP/" rootfs/etc/network/iptables.active

	sed -i "s/%DHCPS%/$DHCPS/; \
			s/%DHCPF%/$DHCPF/; \
			s/%WAN4IP%/$WAN4IP/; \
			s/%WAN6NT%/$WAN6NT/; \
			s/%LAN6NT%/$LAN6NT/; \
			s/%DOMAIN%/$DOMAIN/; \
			s/%LAN4IP%/$LAN4IP/" rootfs/etc/sv/dnsmasq/run

	ln -s -f -T /tmp/resolv.conf rootfs/etc/ppp/resolv.conf

	find rootfs -type f -name .empty -delete

	rm rootfs/THIS_IS_NOT_YOUR_ROOT_FILESYSTEM

	rm rootfs/etc/init.d/S01logging
	rm rootfs/etc/init.d/S50dropbear

	sed -i -e 's/ext2/auto/' rootfs/etc/fstab
	sed -i -e 's/tmpfs/ramfs/' rootfs/etc/fstab
	sed -i -e 's/^devpts/#devpts/' rootfs/etc/fstab

	# misc unneeded bits
	rm -rf rootfs/home/ftp
	rm -rf rootfs/var/lib
	rm -rf rootfs/var/pcmcia
	rm -rf rootfs/usr/share/udhcpc
	rm -rf rootfs/share/man

	rm -f rootfs/usr/sbin/pppdump
	rm -f rootfs/usr/sbin/pppstats
	rm -f rootfs/usr/sbin/chat

	DELETE="minconn passprompt passwordfd winbind openl2tp pppol2tp"
	for D in $DELETE; do
		rm -f rootfs/usr/lib/pppd/2.4.5/$D.so
	done

	rm -f rootfs/lib/modules/*/source
	rm -f rootfs/lib/modules/*/build
	rm -f rootfs/lib/modules/*/modules.*
	rm -f rootfs/usr/lib/tc/*.dist

	cp -a src/buildroot/output/host/usr/mipsel-buildroot-linux-uclibc/lib/libgcc_s.so* rootfs/lib

	find rootfs/lib rootfs/usr/lib -type f \( -name '*.a' -o -name '*.la' \) -delete

	# sstrip everything that remains
	find rootfs/bin rootfs/sbin rootfs/lib rootfs/usr -type f \
		| grep -v '\.\(ko\|bin\)$' \
		| grep -v -e routef -e routel -e rtpr \
		| xargs -n1 src/buildroot/output/host/usr/bin/mipsel-linux-sstrip

	ln -s -f -T ../bin/busybox rootfs/sbin/ip
}

sangam () {
	[ -f rootfs/lib/modules/$BR2_LINUX_KERNEL_VERSION/kernel/drivers/net/tiatm.ko ] && return

	wget -P dl -N http://downloads.openwrt.org/sources/sangam_atm-D7.05.01.00-R1.tar.bz2

	rm -rf src/sangam_atm-D7.05.01.00
	tar -xC src -f dl/sangam_atm-D7.05.01.00-R1.tar.bz2

	find src/openwrt/package/kernel/ar7-atm/patches-D7.05.01.00 -type f -name '*.patch' \
		| sort \
		| xargs -I{} sh -c "patch -p1 -f -d src/sangam_atm-D7.05.01.00 < '{}'"

	patch -p1 -f -d src/sangam_atm-D7.05.01.00 < patches/sangam_atm.patch

	ARCH=mips make -C src/sangam_atm-D7.05.01.00

	mkdir -p rootfs/lib/firmware
	cp src/sangam_atm-D7.05.01.00/ar0700mp.bin rootfs/lib/firmware/ar0700xx.bin
	cp src/sangam_atm-D7.05.01.00/tiatm.ko rootfs/lib/modules/$BR2_LINUX_KERNEL_VERSION/kernel/drivers/net
}

pppoe () {
	[ -x rootfs/sbin/br2684ctl ] && return

	wget -P dl -N --content-disposition http://sourceforge.net/projects/linux-atm/files/latest/download

	rm -rf src/linux-atm-2.5.2
	tar -xC src -f dl/linux-atm-2.5.2.tar.gz

	cd src/linux-atm-2.5.2

	CC="$BASEDIR/src/buildroot/output/host/usr/bin/mipsel-linux-gcc" \
	./configure --prefix="$BASEDIR/rootfs" --with-kernel-headers=$KERNELDIR/include --host=mipsel-linux
	make -C src/lib install
	make -C src/br2684 install

	cd ../../
}

bake () {
	objcopy -S -O srec --srec-forceS3 src/buildroot/output/build/linux-$BR2_LINUX_KERNEL_VERSION/vmlinuz vmlinuz.srec

	tools/srec2bin vmlinuz.srec vmlinuz.bin

	if [ $(wc -c vmlinuz.bin | cut -d' ' -f1) -gt 786432 ]; then
		echo kernel too big
		exit 1
	fi

	/usr/sbin/mkfs.jffs2 -D "$DEVTABLE" -X zlib -x lzo -x rtime -e 65536 -n -p -t -l -d rootfs --squash -o fs.img
	if [ $(wc -c fs.img | cut -d' ' -f1) -gt 3211264 ]; then
		echo filesystem too big
		exit 1
	fi

	( dd if=/dev/zero bs=16 count=1; dd if=vmlinuz.bin bs=786432 conv=sync; cat fs.img ) | tools/addpattern -o firmware-code.bin -p WA21
}

if [ "${1:-}" = "clean" ]; then
	clean
	exit 0
fi

if [ -f local ]; then
	. ./local
else
	echo missing 'local' config file, default created, please edit >&2
	cat <<'EOF' > local
HOSTNAME=host
DOMAIN=example.com
# generate your hash with 'makepasswd --crypt-md5 --clearfrom=-' (default: changeme)
ROOTPASSWD='$1$oKPJuaWf$cq4x3Y1lxL39R31y158pT0'

LAN4IP=192.168.1.1
LAN4SN=255.255.255.0

WAN4IP=203.0.113.1
# put here your /48 IPv6 allocation if you have one,
# like "WAN6NT=2001:db8:beef", otherwise leave blank
WAN6NT=

if [ ! "$WAN6NT" ]; then
	WAN6NT=$(echo $WAN4IP | tr . ' ' | xargs printf '2002:%.2x%.2x:%.2x%.2x')
fi

WAN6IP=$WAN6NT::
# LAN IPv6 subnet to use (WAN6NT:LAN6NT::/64)
LAN6NT=1000

# 'PPPoE' or 'PPPoA'
TYPE=PPPoA
# 'LLC' or 'VCmux'
METHOD=LLC
VPI=0
VCI=38
USER=username
PASS=password

NTP=pool.ntp.org
DHCPS=192.168.1.32
DHCPF=192.168.1.63
EOF
	exit 1
fi

if [ -z "$(find . -maxdepth 1 -name 'ssh.*')" ]; then
	echo missing user accounts, please create some >&2
	exit 1
fi

[ "$METHOD" = "LLC" ] && ENCAP=0 || ENCAP=1

git submodule update --init

BASEDIR="$(pwd)"

ar7flashtools

buildroot

eval $(grep BR2_LINUX_KERNEL_VERSION src/buildroot/.config)
export KERNELDIR="$BASEDIR/src/buildroot/output/build/linux-$BR2_LINUX_KERNEL_VERSION"
export CROSS_COMPILE="$BASEDIR/src/buildroot/output/host/usr/bin/mipsel-linux-"

[ "$TYPE" = "PPPoE" ] && pppoe
sangam

DEVTABLE=$(mktemp)

cat <<'EOF' > "$DEVTABLE"
# <name>			<type>	<mode>	<uid>	<gid>	<major>	<minor>	<start>	<inc>	<count>
/bin/busybox			f	4755	0	0	-	-	-	-	-
EOF

customise

bake

rm "$DEVTABLE"

echo
echo "your firmware is now ready to deploy, do this by typing:"
echo "echo -e \"mode binary\\\nconnect $LAN4IP\\\nput firmware-code.bin\" | tftp"

exit 0
