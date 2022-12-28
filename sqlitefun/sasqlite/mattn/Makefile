all: vet test

vet:
	go vet --tags sqlite_vtable

test:
	AWS_REGION=dummy \
	AWS_ACCESS_KEY_ID=dummy \
	AWS_SECRET_ACCESS_KEY=dummy \
	go test -v -tags sqlite_vtable
