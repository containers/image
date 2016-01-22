export GO15VENDOREXPERIMENT=1

BINDIR=${DESTDIR}/usr/bin/
MANDIR=${DESTDIR}/usr/share/man

all:
	go build -o skopeo .
	go-md2man -in man/skopeo.1.md -out skopeo.1

install:
	install -d -m 0755 ${BINDIR}
	install -m 755 skopeo ${BINDIR}
	install -m 644 skopeo.1 ${MANDIR}/man1/

clean:
	rm -f skopeo
	rm -f skopeo.1
