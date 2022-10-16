EXT = so
ifeq ($(shell uname),Darwin)
	EXT = dylib
endif
LIBS = csv.$(EXT)

.PHONY: all
all: $(LIBS)

.PHONY: clean
clean:
	-rm -f $(LIBS)

%.$(EXT): *.go
	go build -buildmode=c-shared -o $@

run: all
	sqlite3 -cmd '.load csv' -cmd 'create virtual table solar_plants using csv(filename="solar_plants.csv")'

