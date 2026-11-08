#include <string>

namespace geo {

class Shape {
public:
    virtual double area() const = 0;
};

}

static const char *kHelp = R"(
int notAFunction() {
class NotAClass {
)";

static const char *kOther = R"delim(
double alsoNot() {
)delim";

double square(double x) {
    return x * x;
}

/*
double commented(double x) {
    return x;
}
*/

int main(int argc, char **argv) {
    return square(argc) > 0 ? 0 : 1;
}
