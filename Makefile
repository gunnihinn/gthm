BIN := /usr/bin/gthm

gthm: main.go schema.sql check
	CGO_ENABLED=1 go build

$(BIN): gthm
	install $< $@

install: $(BIN)

@PHONY: check
check:
	go test

@PHONY: clean
clean:
	rm -f gthm
