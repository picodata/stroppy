all:
	go mod tidy
	go mod vendor
	go build -o bin/stroppy ./cmd/stroppy

clean:
	rm -rf _data

postgres_pop:
	bin/stroppy pop --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --count 5000 -r 1.02

postgres_pay:
	bin/stroppy pay --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --check --count=100000 -r 1.02

postgres_payz:
	bin/stroppy pay --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --check --count=100000 -z true

fdb_init:
	echo "docker:docker@127.0.0.1:4500" > fdb.cluster

fdb_pop:
	bin/stroppy pop --url fdb.cluster --count 5000 -d fdb -r 1.02

fdb_pay:
	bin/stroppy pay --url fdb.cluster --check --count=100000 -d fdb -r 1.02

fdb_payz:
	bin/stroppy pay --url fdb.cluster --check --count=100000 -d fdb -z true

lint:
	golangci-lint run --new-from-rev=master

deploy_yandex:
	bin/stroppy deploy --cloud yandex --flavor small --nodes 3
