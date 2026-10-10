# TEST_VERBOSE:=1
# TEST_VERBOSE:=
# TEST_RUN:=Test

# bin/* targets
TRG=$(patsubst exe/%,bin/%,$(wildcard exe/*))

# sources for bin/* in exe/* one directory per executable
EXTRA=$(filter-out stuff/% %_test.go,$(wildcard */*.go))

# top-level package directories in the form lsn slot ...
MODS=$(patsubst %/,%,$(sort $(dir $(EXTRA))))

V=$(if $(findstring 1,$(TEST_VERBOSE)),-v)
R=$(if $(TEST_RUN),-run $(TEST_RUN))

# since we are now requiring go1.27.1 jsonv2 is not an experiment anymore
# export GOEXPERIMENT=jsonv2

export OYPG_TEST_CONNINFO=postgres://postgres:pwp@127.0.0.1:5440/test

.PHONY: exe test

test: $(patsubst %,T%,$(MODS))

exe: $(TRG)

bin:
	mkdir -p bin

G%:: %
	go generate ./$<

Gmsg:: msg/common.tmpl msg/gen/gen.go

Gslot:: slot/cfg.go

Tslot:: Gslot

Tmsg:: Gmsg

T%:: %
	@go test $V $R -count=1 -coverprofile $</cover.out ./$<
	@go tool cover -html=$</cover.out -o $</coverage.html
	@echo Coverage report in file://$$PWD/$</coverage.html

bin/%:: bin exe/%/*.go $(EXTRA)
	go build -o $@ ./exe/$(@F)

doc:
	pkgsite -http=127.0.0.1:6060

# define DR
# bin/$1:: dump/$1
# 	cp $$< $$@
# 	chmod 755 $$@
# endef

# $(foreach t,dump restore create-origin check-identity archive_command,$(eval $(call DR,$(t))))

# update, commit and push VERSION before running make tag
tag:	VERSION
	git fetch origin && \
	[ "$$(git branch --show-current)" = master ] && \
	L="$$(git rev-parse HEAD)" && \
	R="$$(git rev-parse refs/remotes/origin/master)" && \
	[ "$$L" = "$$R" ] && \
	T="$$(cat VERSION)" && \
	git tag $$T && \
	git push origin tag $$T && \
	M="$$(sed '/^module/!d; s/^module *//' go.mod)" && \
	GOPROXY=proxy.golang.org go list -m $$M@$$T
