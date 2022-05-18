BUILD_COMMIT=$(shell git rev-list -1 HEAD)
BUILD_DATE=$(shell date +%d-%m-%Y-%H:%M)
VERSION=$(shell build/get_version.sh)

LDFLAGS="-s -w -X main.version=$(VERSION) -X main.commit=$(BUILD_COMMIT) -X main.date=$(BUILD_DATE)"

cache:
	go mod tidy
	go mod vendor

build:
	go build -ldflags=$(LDFLAGS) -o bin/stroppy ./cmd/stroppy
.PHONY: build

all: cache build

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

fmt:
	gofumpt -w -s .

lint:
	go mod vendor
	golangci-lint run

deploy_yandex:
	bin/stroppy deploy --cloud yandex --flavor small --nodes 3

configure_fdb:
	fdbcli -C /var/fdb/fdb.cluster --exec 'configure new single memory'

test: configure_fdb
	current_branch=$(git rev-parse --abbrev-ref HEAD)
	if [ $current_branch != "develop" ]; then (git fetch && git checkout --track origin/develop && git pull && git checkout $current_branch && go mod vendor && golangci-lint run) else (go mod vendor) fi
	go test ./...
