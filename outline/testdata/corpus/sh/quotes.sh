#!/bin/sh
# Quotes span lines in shell: the braces inside them aren't the function's.

count() {
	awk '{
		n++
	}
	END { print n }' "$1"
}

greet() {
	echo "hello
	} still in the string
	"
}

ansi() {
	printf $'it\'s {\n'
}

last() {
	echo done
}
