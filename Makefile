PROG=	fion

SRCS=	fion.c
SRCS+=	event.c
SRCS+=	layout.c
SRCS+=	window.c
SRCS+=	wm.c
SRCS+=	log.c
SRCS+=	dict.c
SRCS+=	tree.c

OBJS=	$(SRCS:.c=.o)

BINDIR=		/usr/local/bin

LDADD+=		-L/usr/X11R6/lib -L/usr/lib/x86_64-linux-gnu -lxcb -lxcb-keysyms -lxcb-icccm

CFLAGS+=	-I.
CFLAGS+=	-I/usr/X11R6/include
CFLAGS+=	-fstack-protector-all
CFLAGS+=	-Wall -Wstrict-prototypes -Wmissing-prototypes
CFLAGS+=	-Wmissing-declarations
CFLAGS+=	-Wshadow -Wpointer-arith -Wcast-qual
CFLAGS+=	-Wsign-compare
CFLAGS+=	-Werror-implicit-function-declaration
#CFLAGS+=	-Werror # during development phase (breaks some archs)

@:	$(OBJS)
	cc $(CFLAGS) -o $(PROG) $(OBJS) $(LDADD)

clean:
	rm -f $(PROG) $(OBJS)
