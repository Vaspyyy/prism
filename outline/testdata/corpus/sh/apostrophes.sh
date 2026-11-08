#!/bin/sh
# Apostrophes that aren't quotes, each with real quotes after it.

usage() {
	cat <<\EOF
Don't run this as root.
EOF
}

build() {
	echo 'building'
}

help() {
	cat <<END-OF-HELP
Don't forget the flags.
END-OF-HELP
}

clean() {
	cd /tmp;# don't care
	rm -f 'out'
}

after() {
	echo 'done'
}
