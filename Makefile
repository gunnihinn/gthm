BIN := /usr/bin/gthm

gthm: main.go schema.sql db/db.go check
	CGO_ENABLED=1 go build

db/db.go: schema.sql query.sql sqlc.yaml
	sqlc generate

$(BIN): gthm
	install $< $@

install: $(BIN)

@PHONY: check
check:
	go test

@PHONY: clean
clean:
	rm -f gthm
