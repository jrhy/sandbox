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
	sqlite3 \
		-cmd '.load csv' \
		-cmd 'create virtual table solar_plants using csv(filename="solar_plants.csv",header="YES")' \
		-cmd "insert into solar_plants values (1,1,1,1,1);" \
		-cmd "update solar_plants set number_of_solar_plants=290 where state='Arizona';" \
		-cmd ".mode table" \
		-cmd "select * from solar_plants;"

