# A CRLF file: the backslash still continues the string.
x = "abc\
def fake():\
"


def real():
    return x
