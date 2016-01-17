export GOPATH:=$(CURDIR)/Godeps/_workspace:$(GOPATH)

BINDIR=${DESTDIR}/usr/local/bin/

all:
	go build -o skopeo .

install:
	install -d -m 0755 ${BINDIR}
	install -m 755 skopeo ${BINDIR}

clean:
	rm -f skopeo
