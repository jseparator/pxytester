# make
build:
	go build -o bin/pxy *.go

run:
	$(MAKE) build
	./bin/pxy -h

clean:
	rm -rvf bin

.DEFAULT_GOAL := build
