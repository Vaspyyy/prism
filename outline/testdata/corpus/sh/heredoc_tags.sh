#!/bin/sh
# A shift that looks like a heredoc doesn't hide a later real one.

calc() {
	a=$((b<<EOF))
}

gen() {
	cat <<-EOF
fake() {
	EOF
}

after() {
	echo ok
}
