all:
	go mod tidy
	go mod vendor
	go build -mod vendor -o bin/stroppy

clean:
	rm -rf _data

postgres_pop:
	bin/stroppy pop --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --count 5000

postgres_pay:
	bin/stroppy pay --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --check --count=100000

fdb_init:
	echo "docker:docker@127.0.0.1:4500" > fdb.cluster

fdb_pop:
	bin/stroppy pop --url fdb.cluster --count 5000 -d fdb

fdb_pay:
	bin/stroppy pay --url fdb.cluster --check --count=100000 -d fdb

lint:
	golangci-lint run --new-from-rev=master
