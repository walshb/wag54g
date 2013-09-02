#!/bin/sh

set -eu

PPPOE=
USAGE="Usage: $(basename $0) [OPTION]

  -e                   enable PPP-over-Ethernet support
  -h                   display this help and exit
"

while getopts eh f
do
	case $f in
	e)	PPPOE=$f;;
	h | \?)	echo "$USAGE"; [ $f = 'h' ] && exit 0 || exit 1;;
	esac
done
shift $(expr $OPTIND - 1)

ar7flashtools () {
	mkdir -p tools

	[ -x tools/srec2bin ] \
		|| gcc -o tools/srec2bin src/openwrt/tools/firmware-utils/src/srec2bin.c
	[ -x tools/addpattern ] \
		|| gcc -o tools/addpattern src/openwrt/tools/firmware-utils/src/addpattern.c
}

patches () {
	mkdir -p patches/linux

	#alex@berk:/usr/src/wag54g/wag54g$ find src/openwrt/target/linux/ar7 -name '*.patch'
	#src/openwrt/target/linux/ar7/patches-3.9/110-flash.patch
	#src/openwrt/target/linux/ar7/patches-3.9/120-gpio_chrdev.patch
	#src/openwrt/target/linux/ar7/patches-3.9/160-vlynq_try_remote_first.patch
	#src/openwrt/target/linux/ar7/patches-3.9/200-free-mem-below-kernel-offset.patch
	#src/openwrt/target/linux/ar7/patches-3.9/300-add-ac49x-platform.patch
	#src/openwrt/target/linux/ar7/patches-3.9/310-ac49x-prom-support.patch
	#src/openwrt/target/linux/ar7/patches-3.9/320-ac49x-mtd-partitions.patch
	#src/openwrt/target/linux/ar7/patches-3.9/500-serial_kludge.patch
	#src/openwrt/target/linux/ar7/patches-3.9/920-ar7part.patch
	#src/openwrt/target/linux/ar7/patches-3.9/925-actiontec_leds.patch
	#src/openwrt/target/linux/ar7/patches-3.9/950-cpmac_titan.patch
	#src/openwrt/target/linux/ar7/patches-3.9/972-cpmac_fixup.patch

	ln -s -f -T ../cmdline-parts.patch patches/linux/linux-digriz.500-cmdline-parts.patch
	#ln -s -f -T ../../src/openwrt/target/linux/ar7/patches-3.9/500-serial_kludge.patch patches/linux/linux-openwrt.500-serial-kludge.patch
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

hostname () {
	echo -n $HOSTNAME > rootfs/etc/hostname

	cat <<EOF > rootfs/etc/hosts
127.0.0.1       localhost
127.1.0.1       $HOSTNAME.$DOMAIN $HOSTNAME

# The following lines are desirable for IPv6 capable hosts
::1     ip6-localhost ip6-loopback
fe00::0 ip6-localnet
ff00::0 ip6-mcastprefix
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
EOF
}

interfaces () {
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

	up	iptables-restore  < /etc/network/iptables.active

	up	sysctl -q -w net.netfilter.nf_conntrack_max=4096

	# bogons (rfc6890)
	up	ip route add unreachable 10.0.0.0/8
	up	ip route add unreachable 100.64.0.0/10
	up	ip route add unreachable 169.254.0.0/16
	up	ip route add unreachable 172.16.0.0/12
	up	ip route add unreachable 192.0.0.0/24
	up	ip route add unreachable 192.0.2.0/24
	up	ip route add unreachable 192.88.99.0/24
	up	ip route add unreachable 192.168.0.0/16
	up	ip route add unreachable 198.18.0.0/15
	up	ip route add unreachable 198.51.100.0/24
	up	ip route add unreachable 203.0.113.0/24
	up	ip route add unreachable 240.0.0.0/4
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
	up	ip route add unreachable 2002::/16
	up	ip route add unreachable fc00::/7

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

	if [ "$WAN6IP" != "${WAN6IP#2002:}" ]; then
		cat <<EOF >> rootfs/etc/network/interfaces

# 6to4 destination optimisation
auto tun6to4
iface tun6to4 inet6 v4tunnel
	address $WAN6IP
	netmask 16
	gateway ::192.88.99.1
	endpoint any
	local $WAN4IP
EOF
	fi
}

ppp () {
	:
	# fixups for real
	#up	ip route delete unreachable 2002::/16
	#up	ip route add 2001::/32 dev ppp0
	# fixups for tunnelled
	#up	ip route delete unreachable 192.88.99.0/24
	#up	ip route add 2001::/32 dev tun6to4
}

customise () {
	rsync -rl overlay/ rootfs

	hostname
	interfaces

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

	cp -a src/buildroot/output/host/usr/mipsel-buildroot-linux-uclibc/lib/libgcc_s.so* rootfs/lib
	./src/buildroot/output/host/usr/bin/mipsel-linux-sstrip rootfs/lib/libgcc_s.so.1
}

sangam () {
	[ -f rootfs/lib/modules/$BR2_LINUX_KERNEL_VERSION/kernel/drivers/net/tiatm.ko ] && return

	wget -P src -N http://downloads.openwrt.org/sources/sangam_atm-D7.05.01.00-R1.tar.bz2

	rm -rf src/sangam_atm-D7.05.01.00
	tar -xC src -f src/sangam_atm-D7.05.01.00-R1.tar.bz2

	find src/openwrt/package/kernel/ar7-atm/patches-D7.05.01.00 -type f -name '*.patch' \
		| xargs -I{} sh -c "patch -p1 -f -d src/sangam_atm-D7.05.01.00 < '{}'"

	patch -p1 -f -d src/sangam_atm-D7.05.01.00 < patches/sangam_atm.patch

	ARCH=mips make -C src/sangam_atm-D7.05.01.00

	mkdir -p rootfs/lib/firmware
	cp src/sangam_atm-D7.05.01.00/ar0700mp.bin rootfs/lib/firmware/ar0700xx.bin
	cp src/sangam_atm-D7.05.01.00/tiatm.ko rootfs/lib/modules/$BR2_LINUX_KERNEL_VERSION/kernel/drivers/net
}

pppoe () {
	[ -x rootfs/sbin/br2684ctl ] && return

	wget -P src -N --content-disposition http://sourceforge.net/projects/linux-atm/files/latest/download

	tar -xC src -f src/linux-atm-2.5.2.tar.gz

	OLDPWD="$(pwd)"

	cd src/linux-atm-2.5.2

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

	DEVTABLE=$(mktemp)

	cat <<'EOF' > "$DEVTABLE"
# <name>	<type>	<mode>	<uid>	<gid>	<major>	<minor>	<start>	<inc>	<count>
/bin/busybox	f	4755	0	0	-	-	-	-	-
EOF

	/usr/sbin/mkfs.jffs2 -D "$DEVTABLE" -X zlib -x lzo -x rtime -e 65536 -n -p -t -l -d rootfs --squash -o fs.img
	if [ $(wc -c fs.img | cut -d' ' -f1) -gt 3211264 ]; then
		echo filesystem too big
		exit 1
	fi

	rm "$DEVTABLE"

	( dd if=/dev/zero bs=16 count=1; dd if=vmlinuz.bin bs=786432 conv=sync; cat fs.img ) | tools/addpattern -o firmware-code.bin -p WA21
}

. ./local

git submodule init
git submodule update

BASEDIR="$(pwd)"

#VERSION_OPENWRT=$(git --git-dir=src/openwrt/.git rev-parse HEAD | cut -c 1-8)

ar7flashtools

#patches

buildroot

eval $(grep BR2_LINUX_KERNEL_VERSION src/buildroot/.config)
export KERNELDIR="$BASEDIR/src/buildroot/output/build/linux-$BR2_LINUX_KERNEL_VERSION"
export CROSS_COMPILE="$BASEDIR/src/buildroot/output/host/usr/bin/mipsel-linux-"

[ "$PPPOE" ] && pppoe
sangam

customise

bake

#echo -e "mode binary\nconnect 192.168.0.1\nput firmware-code.bin" | tftp

exit 0
