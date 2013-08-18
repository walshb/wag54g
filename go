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

	ln -s -f -T ../../src/openwrt/target/linux/ar7/patches-3.9/500-serial_kludge.patch patches/linux/linux-openwrt.500-serial-kludge.patch
}

buildroot () {
	ln -s -f -T ../../config/buildroot.config src/buildroot/.config

	mkdir -p dl
	ln -s -f -t src/buildroot ../../dl

	export UCLIBC_CONFIG_FILE=../../config/uclibc.config 
	export BUSYBOX_CONFIG_FILE=../../config/busybox.config

	make -C src/buildroot oldconfig
	make -C src/buildroot

	rsync -a src/buildroot/output/target/ rootfs
}

customise () {
	rm rootfs/THIS_IS_NOT_YOUR_ROOT_FILESYSTEM
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

git submodule init
git submodule update

BASEDIR="$(pwd)"

#VERSION_OPENWRT=$(git --git-dir=src/openwrt/.git rev-parse HEAD | cut -c 1-8)

mkdir -p tools

ar7flashtools

#patches

buildroot

eval $(grep BR2_LINUX_KERNEL_VERSION src/buildroot/.config)
export KERNELDIR="$BASEDIR/src/buildroot/output/build/linux-$BR2_LINUX_KERNEL_VERSION"
export CROSS_COMPILE="$BASEDIR/src/buildroot/output/host/usr/bin/mipsel-linux-"

[ "$PPPOE" ] && pppoe
sangam

customise

exit 0
