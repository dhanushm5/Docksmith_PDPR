BINARY=docksmith
DOCKSMITH_HOME?=$(CURDIR)/.docksmith

.PHONY: build run clean clean-data clean-all

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)

clean-data:
	rm -rf $(DOCKSMITH_HOME)

clean-all: clean clean-data
