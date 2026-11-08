#ifndef BUFFER_H
#define BUFFER_H

struct buffer;

/* int commented_out(void); */
int buffer_resize(struct buffer *b, unsigned long len);

#endif
