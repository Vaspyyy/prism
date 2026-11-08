// C++14 digit separators aren't character literals.
int big(int x) {
    if (x > 10'000) {
        return 1;
    }
    return 0x7F'FF + 0b1010'0101;
}

char quote() {
    return '\'';
}

int after() {
    return big(1);
}
