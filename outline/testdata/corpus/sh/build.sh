#!/bin/sh

usage() {
	cat <<EOF
usage: build.sh [target]

fake_in_heredoc() {
EOF
}

function clean {
	rm -rf out
}

build() {
	mkdir -p out
}

help() {
	cat <<-'END'
not_in_help() {
	END
	echo $((1 << 2))
	cat <<< "here() {"
}

# not_a_function() {
build
