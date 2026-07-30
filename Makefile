# #TEST_VERBOSE:=1
# #TEST_VERBOSE:=

# bin/* targets
TRG=$(patsubst exe/%,bin/%,$(wildcard exe/*))

# sources for bin/* in exe/* one directory per executable
EXTRA=$(filter-out stuff/% %_test.go,$(wildcard */*.go))

# top-level package directories in the form lsn slot ...
MODS=$(patsubst %/,%,$(sort $(dir $(EXTRA))))

V=$(if $(findstring 1,$(TEST_VERBOSE)),-v)

all: $(patsubst %,^test-%,$(MODS))

# all: $(TRG)

^test-%:: %
	@go test -count=1 -coverprofile $</cover.out $V ./$<
	@go tool cover -html=$</cover.out -o $</coverage.html
	@echo Coverage report in file://$$PWD/$</coverage.html

# # define DR
# # bin/$1:: dump/$1
# # 	cp $$< $$@
# # 	chmod 755 $$@
# # endef

# # $(foreach t,dump restore create-origin check-identity archive_command,$(eval $(call DR,$(t))))

bin/%:: exe/%/*.go $(EXTRA)
	mkdir -p bin && go build -o $@ ./exe/$(@F)
