all:
	go mod tidy
	go mod vendor
	go build -mod vendor -o bin/stroppy

clean:
	rm -rf _data

pop:
	bin/stroppy pop --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --count 5000

pay:
	bin/stroppy pay --url postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable --check --count=100000
