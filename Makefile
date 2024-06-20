BIN := /usr/bin/gthm

gthm: main.go schema.sql pkg/db/db.go check
	CGO_ENABLED=1 go build

pkg/db/db.go: schema.sql query.sql sqlc.yaml
	sqlc generate

pkg/blog/schema.sql: schema.sql
	cp $< $@

$(BIN): gthm
	install $< $@

install: $(BIN)

@PHONY: check
check: pkg/db/db.go pkg/blog/schema.sql
	go test ./...

@PHONY: clean
clean:
	rm -f gthm pkg/blog/schema.sql
