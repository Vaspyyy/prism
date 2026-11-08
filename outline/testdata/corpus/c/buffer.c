#include <stdlib.h>

#define BUFFER_SIZE 64

/*
int commented_out(void) {
    return 0;
}
*/

struct buffer {
    char *data;
    size_t len;
};

typedef struct buffer buffer_t;

static const char *usage = "int not_a_function(void) {";

static const char *long_text = "first part \
int not_a_function_either(void) {";

buffer_t *buffer_new(size_t len)
{
    buffer_t *b = malloc(sizeof *b);
    b->len = len; /* int inline_comment(void) {} */
    return b;
}

void buffer_free(buffer_t *b) {
    free(b);
}

int buffer_resize(buffer_t *b,
                  size_t len) {
    b->len = len;
    return 0;
}
