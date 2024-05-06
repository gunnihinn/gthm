BIN := /usr/bin/gthm

gthm: main.go schema.sql
	CGO_ENABLED=1 go build

$(BIN): gthm
	install $< $@

install: $(BIN)

@PHONY: clean
clean:
	rm -f gthm
