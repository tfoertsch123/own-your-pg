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

export GOEXPERIMENT=jsonv2

all: $(patsubst %,T%,$(MODS)) $(TRG)

bin:
	mkdir -p bin

G%:: %
	go generate ./$<

Gmsg:: msg/common.tmpl msg/gen/gen.go

Gslot:: slot/cfg.go

Tslot:: Gslot

Tmsg:: Gmsg

T%:: %
	@go test -count=1 -coverprofile $</cover.out $V ./$<
	@go tool cover -html=$</cover.out -o $</coverage.html
	@echo Coverage report in file://$$PWD/$</coverage.html

bin/%:: bin exe/%/*.go $(EXTRA)
	go build -o $@ ./exe/$(@F)

# define DR
# bin/$1:: dump/$1
# 	cp $$< $$@
# 	chmod 755 $$@
# endef

# $(foreach t,dump restore create-origin check-identity archive_command,$(eval $(call DR,$(t))))
